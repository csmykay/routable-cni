// Copyright 2021 Hewlett Packard Enterprise Development LP
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"

	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
)

const (
	proxyArp     = "net.ipv4.conf.%s.proxy_arp"
	nonLocalBind = "net.ipv4.ip_nonlocal_bind"
	rtName       = "routable-cni"
	rtTable      = "/etc/iproute2/rt_tables"
	rtIndex      = 10000
)

// NetConf - parameters to be used for routable-cni configuration
// passed in through multus-NAD
type NetConf struct {
	types.NetConf
	// default private interface for the container
	PrivateIf string `json:"private_if"`
	// Interface of the container whose ip address will be made
	// public
	PublicIf string `json:"public_if"`
	// Host interface to be used to advertise ipaddress of the
	// container
	HostIf string `json:"host_if"`
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func fetchDefaultInterface() (string, error) {
	routeToDstIP, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return "", err
	}

	for _, v := range routeToDstIP {
		if v.Dst == nil {
			l, err := netlink.LinkByIndex(v.LinkIndex)
			if err != nil {
				return "", err
			}
			return l.Attrs().Name, nil
		}
	}

	return "", fmt.Errorf("no default route interface found")
}

func loadConf(bytes []byte, envArgs string) (*NetConf, error) {
	netConf := &NetConf{}
	if err := json.Unmarshal(bytes, netConf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	// If host_if is not picked,
	if netConf.HostIf == "" {
		defaultIf, err := fetchDefaultInterface()
		if err != nil {
			return nil, fmt.Errorf("host_if is empty, failed to fetch default interface")
		}
		netConf.HostIf = defaultIf
	}

	// Verify if the interface is present
	_, err := netlink.LinkByName(netConf.HostIf)
	if err != nil {
		return nil, fmt.Errorf("failed to access host_if %q: %v", netConf.HostIf, err)
	}

	if netConf.PrivateIf == "" || netConf.PublicIf == "" {
		return nil, fmt.Errorf("private_if/public_if must not be empty %v:%v", netConf, string(bytes))
	}

	return netConf, nil
}

func checkAndSetSysctlParameters(hostIf string) error {
	ifProxyArp := fmt.Sprintf(proxyArp, hostIf)
	proxyArpVal, err := sysctl.Sysctl(ifProxyArp)
	if err != nil {
		return fmt.Errorf("unable to fetch sysctl parameter for %q: %v", ifProxyArp, err)
	}

	if proxyArpVal != "1" {
		proxyArpVal, err := sysctl.Sysctl(ifProxyArp, "1")
		if err != nil || proxyArpVal != "1" {
			return fmt.Errorf("unable to set sysctl parameter for %q: %v", ifProxyArp, err)
		}
	}

	// Check and enable nonlocal_bind for sending GARP
	binVal, err := sysctl.Sysctl(nonLocalBind)
	if err != nil {
		return fmt.Errorf("unable to fetch sysctl parameter for %q: %v", nonLocalBind, err)
	}

	if binVal != "1" {
		binVal, err := sysctl.Sysctl(nonLocalBind, "1")
		if err != nil || binVal != "1" {
			return fmt.Errorf("unable to set sysctl parameter for %q: %v", nonLocalBind, err)
		}
	}
	return nil
}

func fetchIPAddr(ifName string) (net.IP, error) {
	privateIf, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch interface %q: %v", ifName, err)
	}
	addrs, err := privateIf.Addrs()
	var ip net.IP
	for _, addr := range addrs {
		if ip = addr.(*net.IPNet).IP.To4(); ip != nil {
			break
		}
	}

	if ip == nil {
		return nil, fmt.Errorf("failed to fetch ipaddress for interface %q: %v", ifName, addrs)
	}

	return ip, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	netConf, err := loadConf(args.StdinData, args.Args)
	if err != nil {
		return err
	}

	// Check and create route table entry, we will be using a custom
	// route table to create ip rules.
	data, err := ioutil.ReadFile(rtTable)
	if err != nil {
		return fmt.Errorf("failed to read %q: %v", rtTable, err)
	}

	rtEntry := strconv.Itoa(rtIndex) + " " + rtName
	if !strings.Contains(string(data), rtEntry) {
		file, err := os.OpenFile(rtTable, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file for writing %q: %v", rtTable, err)
		}
		defer file.Close()
		if _, err := file.WriteString(rtEntry + "\n"); err != nil {
			return fmt.Errorf("failed to update %q: %v", rtTable, err)
		}
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	var privateIP, publicIP net.IP
	// Fetch ipaddresses from the namespace for both private and public interfaces
	err = netns.Do(func(_ ns.NetNS) error {
		privateIP, err = fetchIPAddr(netConf.PrivateIf)
		if err != nil {
			return err
		}
		publicIP, err = fetchIPAddr(netConf.PublicIf)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Enable sysctl parameters on the host interface
	err = checkAndSetSysctlParameters(netConf.HostIf)
	if err != nil {
		return err
	}

	netPublicIP := &net.IPNet{
		IP:   publicIP,
		Mask: net.CIDRMask(32, 32),
	}

	// Create the ip route to route public ip through the private ip
	err = netlink.RouteAdd(&netlink.Route{
		Dst:   netPublicIP,
		Gw:    privateIP,
		Table: rtIndex,
	})

	if err != nil {
		return fmt.Errorf("failed to add route entry %v", err)
	}

	//	Create ip rule for public ip
	rule := netlink.NewRule()
	rule.Dst = netPublicIP
	rule.Table = rtIndex

	err = netlink.RuleAdd(rule)

	if err != nil {
		return fmt.Errorf("failed to add rule entry %v", err)
	}

	_ = arping.GratuitousArpOverIfaceByName(publicIP, netConf.HostIf)

	result := &current.Result{CNIVersion: netConf.CNIVersion}
	return types.PrintResult(result, netConf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	netConf, err := loadConf(args.StdinData, args.Args)
	if err != nil {
		return nil
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil
	}
	defer netns.Close()

	var publicIP net.IP

	// Fetch ipaddresses from the namespace for both private and public interfaces
	err = netns.Do(func(_ ns.NetNS) error {
		publicIP, err = fetchIPAddr(netConf.PublicIf)
		if err != nil {
			return err
		}

		return nil
	})

	// If we can't fetch ipaddress there is nothing to do
	if err != nil {
		return nil
	}

	netPublicIP := &net.IPNet{
		IP:   publicIP,
		Mask: net.CIDRMask(32, 32),
	}

	netlink.RouteDel(&netlink.Route{
		Dst:   netPublicIP,
		Table: rtIndex,
	})

	rule := netlink.NewRule()
	rule.Dst = netPublicIP
	rule.Table = rtIndex

	err = netlink.RuleDel(rule)

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	// NOT IMPLEMENTED
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("routable-cni"))
}
