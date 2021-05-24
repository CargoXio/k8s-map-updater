package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

const DefaultTemplatePath = "/opt/mapudater/template.tpl"
const TemplatePathEnvVar = "TEMPLATE_PATH"
var TemplatePath = ""

const DefaultConfigName = "redis-haproxy"
const ConfigNameEnvVar = "CONFIG_NAME"
var ConfigName = ""

const DefaultKeyName = "haproxy.cfg"
const KeyNameEnvVar = "KEY_NAME"
var KeyName = ""

func getTemplate() (*template.Template, error) {
	b, err := ioutil.ReadFile(TemplatePath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not read file: %s", TemplatePath)
	}

	tpl, err := template.New("config").Parse(string(b))
	if err != nil {
		return nil, errors.Wrapf(err, "Could not parse template: %s", TemplatePath)
	}

	return tpl, nil
}

func applyTemplate(client kubernetes.Interface, namespace string) {
	tpl, err := getTemplate()
	if err != nil {
		logrus.WithError(err).Error("Could not read template: %v", err)
		return
	}

	pods, err := client.CoreV1().Pods(namespace).List(context.TODO(),  metav1.ListOptions{})
	if err != nil {
		logrus.WithError(err).Error("Could not get pods in namespace %s: %v", namespace, err)
		return
	}

	logrus.Infof("Applying template %s to %v", TemplatePath, pods)

	var out = &bytes.Buffer{}
	err = tpl.Execute(out, &struct{
		pods *v1.PodList
	}{
		pods: pods,
	})
	if err != nil {
		logrus.WithError(err).Error("Error executing template %v: %v", TemplatePath, err)
		return
	}

	logrus.Infof("Patching configmap %s/%s -> %s", namespace, ConfigName, KeyName)

	payload := []struct{
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}{{
		Op:    "replace",
		Path:  fmt.Sprintf("/data/%s", KeyName),
		Value: out.String(),
	}}
	data, err := json.Marshal(payload)
	if err != nil {
		logrus.WithError(err).Error("Failed converting patch object to JSON: ", err)
		return
	}

	_, err = client.CoreV1().ConfigMaps(namespace).Patch(
		context.TODO(),
		ConfigName,
		types.JSONPatchType,
		data,
		metav1.PatchOptions{},

		)
	if err != nil {
		logrus.WithError(err).Error("Failed patching map %s/%s: ", namespace, ConfigName, err)
		return
	}

}

func watchPods(client kubernetes.Interface, namespace string, store cache.Store) cache.Store {
	//Define what we want to look for (Pods)
	resyncPeriod := 30 * time.Minute
	//Setup an informer to call functions when the watchlist changes
	store, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Pods(namespace).List(context.TODO(), lo)
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Pods(namespace).Watch(context.TODO(), lo)
			},
		},
		&v1.Pod{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				logrus.Debug("Pod created: %s", pod.ObjectMeta.Name)
				applyTemplate(client, namespace)
			},
			UpdateFunc: func(old interface{}, obj interface{}) {
				pod := obj.(*v1.Pod)
				logrus.Debug("Pod updated: %s", pod.ObjectMeta.Name)
				applyTemplate(client, namespace)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				logrus.Debug("Pod deleted: %s", pod.ObjectMeta.Name)
				applyTemplate(client, namespace)
			},
		},
	)

	//Run the controller as a goroutine
	go controller.Run(wait.NeverStop)
	return store
}

func watchConfig(client kubernetes.Interface, namespace string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				logrus.Debug("event: %v", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					logrus.Infof("modified file: %s", event.Name)
					applyTemplate(client, namespace)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(TemplatePath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not watch file: %s", TemplatePath)
	}

	return watcher, nil
}

func main() {
	var config *rest.Config
	var err error
	var client kubernetes.Interface

	config, err = rest.InClusterConfig()
	if err != nil {
		panic(errors.WithStack(err))
	}

	ns, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		panic(errors.WithStack(err))
	}

	//Create a new client to interact with cluster and freak if it doesn't work
	client = kubernetes.NewForConfigOrDie(config)

	TemplatePath = getFromEnv(TemplatePathEnvVar, DefaultTemplatePath)
	ConfigName = getFromEnv(ConfigNameEnvVar, DefaultConfigName)
	KeyName = getFromEnv(KeyNameEnvVar, DefaultKeyName)

	_, err = getTemplate()
	if err != nil {
		panic(errors.WithStack(err))
	}

	//Create a cache to store Pods
	var podsStore cache.Store
	//Watch for Pods
	podsStore = watchPods(client, string(ns), podsStore)

	_, err = watchConfig(client, string(ns))
	if err != nil {
		panic(errors.WithStack(err))
	}


	http.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("OK"))
		return
	})
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(errors.WithStack(err))
	}

}

func getFromEnv(envVar string, defaultValue string) (res string) {
	res = strings.TrimSpace(os.Getenv(envVar))
	if res == "" {
		res = defaultValue
	}
	return
}
