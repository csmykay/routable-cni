#!/bin/sh
#
# Copyright 2021 Hewlett Packard Enterprise Development LP
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# Always exit on errors.
set -e

# Set known directories.
CNI_BIN_DIR="/host/opt/cni/bin"
SRC_BIN_FILE="/usr/bin/routable-cni"

CNI_ETC_DIR="/host/etc/cni/net.d"
NET_CONF_JSON="/config/net-conf.json"

# Give help text for parameters.
usage()
{
    printf "Copy the cni binary and configuration file to filessystem on host\n"
    printf "\n"
    printf "./entrypoint.sh\n"
    printf "\t-h --help\n"
    printf "\t--cni-bin-dir=%s\n" $CNI_BIN_DIR
    printf "\t--cni-etc-dir=%s\n" $CNI_ETC_DIR
    printf "\t--conf-file=%s\n" $NET_CONF_JSON
}

# Parse parameters given as arguments to this script.
while [ "$1" != "" ]; do
    PARAM=$(echo "$1" | awk -F= '{print $1}')
    VALUE=$(echo "$1" | awk -F= '{print $2}')
    case $PARAM in
        -h | --help)
            usage
            exit
        ;;
        --cni-bin-dir)
            CNI_BIN_DIR=$VALUE
        ;;
        --cni-etc-dir)
            CNI_ETC_DIR=$VALUE
        ;;
        --conf-file)
            NET_CONF_JSON=$VALUE
        ;;
        *)
            /bin/echo "ERROR: unknown parameter \"$PARAM\""
            usage
            exit 1
        ;;
    esac
    shift
done


# Loop through and verify each location each.
for i in $CNI_BIN_DIR $SRC_BIN_FILE $CNI_ETC_DIR $NET_CONF_JSON
do
    if [ ! -e "$i" ]; then
        /bin/echo "File/Directory $i not present"
        exit 1;
    fi
done

# Copy the binary and configuration to destination directories
cp -f "$SRC_BIN_FILE" "$CNI_BIN_DIR"
cp -f "$NET_CONF_JSON" "$CNI_ETC_DIR/90-routable-cni.conf"


sleep infinity
