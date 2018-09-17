package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/ncw/swift"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type k8s struct {
	clientset kubernetes.Interface
}

var (
	// default resourceQuota limits
	hardPods           = resource.NewQuantity(5, resource.DecimalSI)
	hardRequestsCPU    = resource.NewQuantity(1, resource.DecimalSI)
	hardRequestsMemory = resource.NewQuantity(1*1024*1024*1024, resource.BinarySI)
	hardLimitsCPU      = resource.NewQuantity(5, resource.DecimalSI)
	hardLimitsMemory   = resource.NewQuantity(5*1024*1024*1024, resource.BinarySI)
)

func main() {
	// setup gin router
	router := gin.Default()

	router.Use(static.Serve("/", static.LocalFile("./views", true)))

	api := router.Group("/api/v1")
	{
		api.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})
		api.GET("/list", ListNamespacesHandler)
		api.POST("/create/:namespace", CreateNamespaceHandler)
		api.GET("/status/:namespace", StatusNamespaceHandler)
	}

	// start this thing!
	router.Run(":3000")
}

// CreateKubernetesClient creates kubernetes client
func CreateKubernetesClient() (*k8s, error) {
	// load kubernetes configuration
	kubeconfig := filepath.Join(
		os.Getenv("HOME"), ".kube", "config",
	)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes configuration: %v\n", err)
		return nil, err
	}

	client := k8s{}
	client.clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes configuration: %v\n", err)
		return nil, err
	}

	return &client, nil
}

// NameSpaceExists checks if a namespace exists
func NameSpaceExists(namespace string) bool {
	kubernetesAPI, err := CreateKubernetesClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
	}

	getOptions := metav1.GetOptions{}

	_, err = kubernetesAPI.clientset.CoreV1().Namespaces().Get(namespace, getOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to retrieve namespace: %v\n", err)
		return false
	}

	return true
}

// ListNamespacesHandler lists all namespaces
func ListNamespacesHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	kubernetesAPI, err := CreateKubernetesClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
	}

	listOptions := metav1.ListOptions{}
	namespaces, err := kubernetesAPI.clientset.CoreV1().Namespaces().List(listOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to retrieve namespaces: %v\n", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": namespaces,
	})
}

// CreateNamespaceHandler creates a namespace with the default resource quota
func CreateNamespaceHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	// take the namespace parameter
	namespace := c.Param("namespace")

	message := "Failed to create namespace"
	if NameSpaceExists(namespace) {
		message = "Namespace already exists"
	} else {
		createStatus := true
		const (
			ResourcePods           v1.ResourceName = "pods"
			ResourceRequestsCPU    v1.ResourceName = "requests.cpu"
			ResourceRequestsMemory v1.ResourceName = "requests.memory"
			ResourceLimitsCPU      v1.ResourceName = "limits.cpu"
			ResourceLimitsMemory   v1.ResourceName = "limits.memory"
		)

		kubernetesAPI, err := CreateKubernetesClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
			createStatus = false

		}

		// create Namespace
		namespaceConfiguration := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_, err = kubernetesAPI.clientset.CoreV1().Namespaces().Create(namespaceConfiguration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create Kubernetes namespace: %v\n", err)
			createStatus = false
		}

		// create ResourceQuota
		resourceQuotaConfiguration := &v1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
			Spec: v1.ResourceQuotaSpec{Hard: v1.ResourceList{ResourcePods: *hardPods,
				ResourceRequestsCPU:    *hardRequestsCPU,
				ResourceRequestsMemory: *hardRequestsMemory,
				ResourceLimitsCPU:      *hardLimitsCPU,
				ResourceLimitsMemory:   *hardLimitsMemory}}}
		_, err = kubernetesAPI.clientset.CoreV1().ResourceQuotas(namespace).Create(resourceQuotaConfiguration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create Kubernetes resourceQuota: %v\n", err)
			createStatus = false
		}

		_, err = StoreNamespaceConfigInSwift(namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to store Kubernetes configuration in Swift: %v\n", err)
			createStatus = false
		}

		if createStatus {
			message = "Namespace created successfully"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}

// StatusNamespaceHandler returns the status and usage of a namespace
func StatusNamespaceHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	// take the namespace parameter
	namespace := c.Param("namespace")

	message := ""

	if !NameSpaceExists(namespace) {
		message = "Namespace does not exists"
	} else {
		kubernetesAPI, err := CreateKubernetesClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
		}

		// get resourceQuota
		getOptions := metav1.GetOptions{}
		_, err = kubernetesAPI.clientset.CoreV1().ResourceQuotas(namespace).Get(namespace, getOptions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create Kubernetes resourceQuota: %v\n", err)
		}
		message = "Not yet implemented"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}

// StoreNamespaceConfigInSwift creates Kubernetes namespace and resourcequota
// configuration and stores it in Openstack's swift
func StoreNamespaceConfigInSwift(namespace string) (status string, err error) {
	// create yaml file
	namespaceTemplate, err := ioutil.ReadFile("namespaceTemplate.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open template file: %v\n", err)
		return "Failed", err
	}
	namespaceYaml := strings.Replace(string(namespaceTemplate), "namespaceName", namespace, -1)

	// create the connection
	c := swift.Connection{
		UserName: "test:tester",
		ApiKey:   "testing",
		AuthUrl:  "http://127.0.0.1:12345/auth/v1.0",
	}
	// authenticate
	authenciationErr := c.Authenticate()
	if authenciationErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to log into Swift: %v\n", authenciationErr)
		return "Failed", err
	}

	containerName := "swift"
	objectName := namespace + "-namespace.yaml"

	err = c.ObjectPutString(containerName, objectName, namespaceYaml, "text/plain")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open Swift writer: %v\n", err)
		return "Failed", err
	}

	return "Configuration stored in Swift", nil
}
