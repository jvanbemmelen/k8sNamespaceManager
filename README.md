# k8sNamespaceManager
Web app that creates and manages Kubernetes name spaces. It creates a namespace and default resource quotas.

Intended flow:
- user logs into web GUI using LDAP credentials validated through DEX
- namespace name will default to dev-$authenticated_username
- if namespace already exists, show the status and usage of the namespace
- if namespace does not exist, create it on user request (push button)
- namespace and resourcequota are configured on the kubernetes cluster
- namespace and resourcequota yaml configurations are stored on Swift
- on successfull creation and storage the GUI confirms the namespace
- a cron job takes the yaml configurations from Swift and creates a PR in the Kubernetes namespace Github repo. These files allow for future management of the namespaces: changing resource quotas, DR etc.


Current default resource quotas:
```
Resource Quotas
 Name:            test01
 Resource         Used  Hard
 --------         ---   ---
 limits.cpu       0     5
 limits.memory    0     5Gi
 pods             0     5
 requests.cpu     0     1
 requests.memory  0     1Gi
```

Missing:
- authentication
- GUI
- k8s cronjob that takes objects from swift and stores them in github


##Usage
Create namespace test1:
```
curl -XPOST http://localhost:3000/api/v1/create/test1
```

## Installation
To build the container:
```
./build.sh
```

Run the container (remove the minikube volume if not using minikube):
```
docker run -ti -v $HOME/.kube:/.kube/ -v $HOME/.minikube/:$HOME/.minikube/ \
-e "SWIFT_USER_NAME=test:tester" -e "SWIFT_API_KEY=tester" \
-e "SWIFT_AUTH_URL=http://192.168.0.11:12345/auth/v1.0" -e "SWIFT_CONTAINER_NAME=kubernetes" \
-p 3000:3000 k8s_namespace_manager
```

If you do not have access to a Swift cluster, try [this](https://hub.docker.com/r/morrisjobke/docker-swift-onlyone/) for Swift in a container.


###Environment variables:

Variable name | Description
------------- | -----------
SWIFT_USER_NAME | user name to use for Swift
SWIFT_API_KEY | api key to use for Swift
SWIFT_AUTH_URL | auth url for Swift
SWIFT_CONTAINER_NAME | name of the Swift container
