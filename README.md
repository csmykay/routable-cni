# Summary
The routable CNI is a Container Network Interface (CNI) plugin designed to make secondary network interface for a container
to be routable through the primary network interface of the container. It is designed to work with [Multus] (https://github.com/intel/multus-cni)


# Build & Clean

- Set a custom TAG for the building the routable-cni docker image. This is done by creating a Local.mk file

TAG="bluedata/routable-cni:my-dev"

- Building the image
```
make image
```
This will build the docker image containing the routable-cni binary and will also generate images/routable-cni-ds.yaml file the appropriate image tag that is to be used for deployment.

- Pushing the image
```
make push
```

- Cleaning
```
make clean
```


# Deployment
This CNI will be deployed as a daemonset.

## Development deploy
kubectl apply -f ./images/routable-cni-ds.yaml

## Latest Release version deployment
kubectl apply -f ./releases/routable-cni-ds.yaml


# Multus Network Attachment Definition
* `type` (string, required): "routable-cni"
* `name` (string, required): Name of the network
* `host_if` (string, optional): Interface to use on the base host to advertise route for the container. If a value is not provided, cni will detect default routable interface on the host.
* `private_if` (string, required): Primary network interface of the container. Typically eth0.
* `public_if` (string, required): Interface name inside the container whose ipaddress will be made routable.


# Sample Deployment using Static IPAM with macvlan as the secondary interface

## Deploy multus in the cluster

```
kubectl apply -f https://raw.githubusercontent.com/intel/multus-cni/master/images/multus-daemonset.yml
```

## Create network attachment definition to attach a secondary interface to a pod. This example uses macvlan

Deploy macvlan network attachment defintion. "master" interface will have to match the host interface. If left
empty, macvlan driver will use the default interface

```
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-static
  namespace: default
spec:
  config: '{
    "cniVersion": "0.3.1",
    "type": "macvlan",
    "capabilities": { "ips": true },
    "master": "ens32",
    "mode": "bridge",
    "ipam": {
      "type": "static"
      }
  }'
```

```
kubectl apply -f ./test/macvlan-nad.yaml
```


Deploy routable-cni network attachment defintion. Make necessary changes to match

```
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: routable-cni
  namespace: default
spec:
  config: '{
    "cniVersion": "0.3.1",
    "type": "routable-cni",
    "host_if": "ens32",
    "private_if" : "eth0",
    "public_if" : "net0"
  }'
```

```
kubectl apply -f ./test/routable-cni-nad.yaml
```

Deploy pods with custom network attachment. Note the static ipaddress that is being used. This ipaddress
has to be available as a routable network ip in the environment

```
apiVersion: v1
kind: Pod
metadata:
  name: dnsutils-1
  annotations:
    k8s.v1.cni.cncf.io/networks: '[{
        "name" : "macvlan-static",
        "interface" : "net0",
        "ips" : ["16.143.22.232/26"]
      },
      {
        "name" : "routable-cni"
      }
    ]'
spec:
  containers:
  - name: dnsutils-1
    image: bluedata/dnsutils:1.0
    command:
      - sleep
      - "3600"
    imagePullPolicy: IfNotPresent
  restartPolicy: Always
```

```
kubectl apply -f ./test/pods.yaml
```
## FIN