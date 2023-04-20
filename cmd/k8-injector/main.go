package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/expediagroup/kubernetes-sidecar-injector/pkg/inject"
	"github.com/expediagroup/kubernetes-sidecar-injector/pkg/version"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Define environment variables used in Secrets Provider config

func main() {
	var parameters inject.WebhookServerParameters
	// Reset flag package to avoid pollution by glog, which is an indirect dependency
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// retrieve command line parameters
	flag.IntVar(&parameters.Port, "port", 443, "Webhook server port.")
	flag.StringVar(&parameters.CertFile,
		"tlsCertFile",
		"/etc/mutator/certs/cert.pem",
		"Path to file containing the x509 Certificate for HTTPS.",
	)
	flag.StringVar(&parameters.KeyFile,
		"tlsKeyFile",
		"/etc/mutator/certs/key.pem",
		"Path to file containing the x509 Private Key for HTTPS.",
	)
	flag.StringVar(&parameters.InjectName, "injectName", "inject", "Injector Name")
	flag.StringVar(&parameters.InjectPrefix, "injectPrefix", "injector.server-lab.info", "Injector Prefix")
	flag.StringVar(&parameters.InjectConfigMapName, "configName", "config", "ConfigMap Name")
	flag.StringVar(&parameters.SidecarDataKey, "sidecarDataKey", "sidecars.yaml", "ConfigMap Sidecar Data Key")
	// Flag.parse only covers `-version` flag but for `version`, we need to explicitly
	// check the args
	showVersion := flag.Bool("version", false, "Show current version")
	flag.Parse()
	// Either the flag or the arg should be enough to show the version
	if *showVersion || flag.Arg(0) == "version" {
		log.Printf("k8-injector v%s\n", version.Get())

		return
	}

	log.Printf("k8-injector v%s starting up...", version.Get())
	client, err := CreateClient()
	if err != nil {
		log.Printf("Failed to create k8 client : %v", err)
		os.Exit(1)
	}

	whsvr := &inject.WebhookServer{
		Params: parameters,
		Server: &http.Server{
			Addr:              fmt.Sprintf(":%v", parameters.Port),
			TLSConfig:         nil,
			ReadHeaderTimeout: 3 * time.Second,
		},
		K8sClient: client,
	}
	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.Serve)
	mux.HandleFunc("/healthz", whsvr.Health)
	whsvr.Server.Handler = mux
	// start webhook server in goroutine
	go func() {
		log.Printf("Serving mutating admission webhook on %s", whsvr.Server.Addr)
		startServer := func() error {
			return whsvr.Server.ListenAndServeTLS(
				parameters.CertFile,
				parameters.KeyFile,
			)
		}

		if err = startServer(); err != nil {
			log.Printf("Failed to listen and serve: %v", err)
			os.Exit(1)
		}
	}()
	// listen for OS shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Printf("Received OS shutdown signal, shutting down webhook server gracefully...")
	err = whsvr.Server.Shutdown(context.Background())
	if err != nil {
		log.Printf("Failed to shutdown web server %v", err)
	}
}

// CreateClient Create the server.
func CreateClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "error setting up cluster config")
	}

	return kubernetes.NewForConfig(config)
}
