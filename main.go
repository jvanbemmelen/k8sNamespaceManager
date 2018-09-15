package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	jwtmiddleware "github.com/auth0/go-jwt-middleware"
	jwt "github.com/dgrijalva/jwt-go"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Response struct {
	Message string `json:"message"`
}

type Jwks struct {
	Keys []JSONWebKeys `json:"keys"`
}

type JSONWebKeys struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

type k8s struct {
	clientset kubernetes.Interface
}

var (
	kubernetesConfig = "/home/jvanbemmelen/.kube/config"
)

var jwtMiddleWare *jwtmiddleware.JWTMiddleware

func main() {
	jwtMiddleware := jwtmiddleware.New(jwtmiddleware.Options{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			aud := os.Getenv("AUTH0_API_AUDIENCE")
			checkAudience := token.Claims.(jwt.MapClaims).VerifyAudience(aud, false)
			if !checkAudience {
				return token, errors.New("Invalid audience.")
			}
			// verify iss claim
			iss := os.Getenv("AUTH0_DOMAIN")
			checkIss := token.Claims.(jwt.MapClaims).VerifyIssuer(iss, false)
			if !checkIss {
				return token, errors.New("Invalid issuer.")
			}

			cert, err := getPemCert(token)
			if err != nil {
				log.Fatalf("could not get cert: %+v", err)
			}

			result, _ := jwt.ParseRSAPublicKeyFromPEM([]byte(cert))
			return result, nil
		},
		SigningMethod: jwt.SigningMethodRS256,
	})

	jwtMiddleWare = jwtMiddleware
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
		api.GET("/list", authMiddleware(), ListNamespacesHandler)
		api.POST("/create/:namespace", authMiddleware(), CreateNamespaceHandler)
	}

	// start this thing!
	router.Run(":3000")
}

// getPemCert
func getPemCert(token *jwt.Token) (string, error) {
	cert := ""
	resp, err := http.Get(os.Getenv("AUTH0_DOMAIN") + ".well-known/jwks.json")
	if err != nil {
		return cert, err
	}
	defer resp.Body.Close()

	var jwks = Jwks{}
	err = json.NewDecoder(resp.Body).Decode(&jwks)

	if err != nil {
		return cert, err
	}

	x5c := jwks.Keys[0].X5c
	for k, v := range x5c {
		if token.Header["kid"] == jwks.Keys[k].Kid {
			cert = "-----BEGIN CERTIFICATE-----\n" + v + "\n-----END CERTIFICATE-----"
		}
	}

	if cert == "" {
		return cert, errors.New("unable to find appropriate key")
	}

	return cert, nil
}

// authMiddleware intercepts the requests, and check for a valid jwt token
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the client secret key
		err := jwtMiddleWare.CheckJWT(c.Writer, c.Request)
		if err != nil {
			// Token not found
			fmt.Println(err)
			c.Abort()
			c.Writer.WriteHeader(http.StatusUnauthorized)
			c.Writer.Write([]byte("Unauthorized"))
			return
		}
	}
}

// CreateKubernetesClient creates kubernetes client
func CreateKubernetesClient() (*k8s, error) {
	// load kubernetes configuration
	kubeconfig := filepath.Join(
		os.Getenv("HOME"), ".kube", "config",
	)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	client := k8s{}
	client.clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &client, nil
}

// NameSpaceExists checks if a namespace exists
func NameSpaceExists(namespace string) bool {
	kubernetesAPI, err := CreateKubernetesClient()
	if err != nil {
		log.Fatal(err)
	}

	getOptions := metav1.GetOptions{}

	_, err = kubernetesAPI.clientset.CoreV1().Namespaces().Get(namespace, getOptions)
	if err != nil {
		return false
	}

	return true
}

// ListNamespacesHandler lists all namespaces
func ListNamespacesHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	kubernetesAPI, err := CreateKubernetesClient()
	if err != nil {
		log.Fatal(err)
	}

	listOptions := metav1.ListOptions{}
	namespaces, err := kubernetesAPI.clientset.CoreV1().Namespaces().List(listOptions)
	if err != nil {
		log.Fatal(err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": namespaces,
	})
}

// CreateNamespaceHandler returns the requested namespaces
func CreateNamespaceHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	// take the namespace parameter
	namespace := c.Param("namespace")

	kubernetesAPI, err := CreateKubernetesClient()
	if err != nil {
		log.Fatal(err)
	}

	namespaceSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

	_, err = kubernetesAPI.clientset.CoreV1().Namespaces().Create(namespaceSpec)
	if err != nil {
		log.Fatal(err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "created",
	})
}
