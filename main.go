package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	k8s_runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"math"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/getsentry/sentry-go"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

const DefaultTemplatePath = "/opt/mapupdater/template.tpl"
const TemplatePathEnvVar = "TEMPLATE_PATH"

var TemplatePath = ""

const DefaultConfigName = "redis-haproxy"
const ConfigNameEnvVar = "CONFIG_NAME"

var ConfigName = ""

const DefaultKeyName = "haproxy.cfg"
const KeyNameEnvVar = "KEY_NAME"

var KeyName = ""

const DefaultLabelSelector = ""
const LabelSelectorEnvVar = "LABEL_SELECTOR"

var LabelSelector = ""

// ContextHook ...
type ContextHook struct{}

// Levels ...
func (hook ContextHook) Levels() []log.Level {
	return log.AllLevels
}

// Fire ...
func (hook ContextHook) Fire(entry *log.Entry) error {
	if pc, file, line, ok := runtime.Caller(10); ok {
		funcName := runtime.FuncForPC(pc).Name()

		entry.Data["file"] = path.Base(file)
		entry.Data["line"] = line
		entry.Data["func"] = path.Base(funcName)
	}

	return nil
}

func getPodNames(pods []v1.Pod) []string {
	res := make([]string, 0)
	for _, pod := range pods {
		res = append(res, pod.Name)
	}
	return res
}

func toFloat64(ifc interface{}) float64 {
	if ifc == nil {
		return 0.0
	}
	if i, ok := ifc.(int); ok {
		return float64(i)
	} else if i, ok := ifc.(int64); ok {
		return float64(i)
	} else if i, ok := ifc.(int8); ok {
		return float64(i)
	} else if i, ok := ifc.(float32); ok {
		return float64(i)
	} else if i, ok := ifc.(float64); ok {
		return i
	} else if i, ok := ifc.(string); ok {
		if i == "" {
			return 0.0
		}
		f, err := strconv.ParseFloat(i, 64)
		if err != nil {
			panic(errors.Wrapf(err, "Could not convert %q to float64: %v", i, err))
		}
		return f
	} else if i, ok := ifc.(fmt.Stringer); ok {
		return toFloat64(i.String())
	} else {
		panic(errors.Errorf("Could not convert %q to float64!", ifc))
	}
}

func getTemplate() (*template.Template, error) {
	b, err := ioutil.ReadFile(TemplatePath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not read file: %s", TemplatePath)
	}

	tpl, err := template.New("config").Funcs(template.FuncMap{
		"filterByLabel": func(pods []v1.Pod, label string) (res []v1.Pod) {
			res = make([]v1.Pod, 0)

			var key, val string
			if strings.Contains(label, ":") {
				v := strings.SplitN(label, ":", 2)
				key = strings.TrimSpace(v[0])
				val = strings.TrimSpace(v[1])
			} else if strings.Contains(label, "=") {
				v := strings.SplitN(label, "=", 2)
				key = strings.TrimSpace(v[0])
				val = strings.TrimSpace(v[1])
			} else {
				key = strings.TrimSpace(label)
				val = ""
			}

		outer:
			for _, pod := range pods {
				for k, v := range pod.Labels {
					if k == key {
						if val == "" || v == val {
							res = append(res, pod)
							continue outer
						}
					}
				}
			}

			log.Debugf("Filtered pods by label %s: %v", label, getPodNames(res))
			return
		},
		"ceil": func(num interface{}) int64 {
			return int64(math.Ceil(toFloat64(num)))
		},
		"sum": func(num1, num2 interface{}) float64 {
			return toFloat64(num1) + toFloat64(num2)
		},
		"div": func(num1, num2 interface{}) float64 {
			n1 := toFloat64(num1)
			n2 := toFloat64(num2)
			return n1 / n2
		},
	}).Parse(string(b))
	if err != nil {
		return nil, errors.Wrapf(err, "Could not parse template: %s", TemplatePath)
	}

	return tpl, nil
}

func toCRLF(src []byte) string {
	size := len(src)
	out := make([]byte, size)
	pos := 0
	for i, c := range src {
		if c == '\n' {
			// check if previous byte was \r
			if i == 0 || src[i-1] != '\r' {
				size++
				out = append(out, '\000')
				out[pos] = '\r'
				pos++
			}
		}
		out[pos] = c
		pos++
	}

	return string(out[0:size])
}

func applyTemplate(client kubernetes.Interface, namespace string) {
	tpl, err := getTemplate()
	if err != nil {
		log.WithError(err).Errorf("Could not read template: %v", err)
		return
	}

	lo := metav1.ListOptions{}
	if LabelSelector != "" {
		lo.LabelSelector = LabelSelector
	}

	pods, err := client.CoreV1().Pods(namespace).List(context.TODO(), lo)
	if err != nil {
		log.WithError(err).Errorf("Could not get pods in namespace %s: %v", namespace, err)
		return
	}

	log.Infof("Applying template %s to %v", TemplatePath, getPodNames(pods.Items))

	var out = &bytes.Buffer{}
	err = tpl.Execute(out, &struct {
		Pods []v1.Pod
	}{
		Pods: pods.Items,
	})
	if err != nil {
		log.WithError(err).Errorf("Error executing template %v: %v", TemplatePath, err)
		return
	}

	log.Infof("Patching configmap %s/%s -> %s", namespace, ConfigName, KeyName)

	cfg := v1.ConfigMap{
		Data: map[string]string{
			"KeyName": toCRLF(out.Bytes()),
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		log.WithError(err).Errorf("Failed converting patch object to JSON: %v", err)
		return
	}

	_, err = client.CoreV1().ConfigMaps(namespace).Patch(
		context.TODO(),
		ConfigName,
		types.StrategicMergePatchType,
		data,
		metav1.PatchOptions{},
	)

	/*
			payload := []struct {
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
			log.WithError(err).Errorf("Failed converting patch object to JSON: %v", err)
				return
			}

			_, err = client.CoreV1().ConfigMaps(namespace).Patch(
				context.TODO(),
				ConfigName,
				types.JSONPatchType,
				data,
				metav1.PatchOptions{},
			)
	*/
	if err != nil {
		log.WithError(err).Errorf("Failed patching map %s/%s: %v", namespace, ConfigName, err)
		return
	}

	log.Debugf("ConfigMap %s/%s patched", namespace, ConfigName)
}

func watchPods(client kubernetes.Interface, namespace string, store cache.Store) cache.Store {
	//Define what we want to look for (Pods)
	resyncPeriod := 30 * time.Minute
	//Setup an informer to call functions when the watchlist changes
	store, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (k8s_runtime.Object, error) {
				if LabelSelector != "" {
					lo.LabelSelector = LabelSelector
				}
				return client.CoreV1().Pods(namespace).List(context.TODO(), lo)
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				if LabelSelector != "" {
					lo.LabelSelector = LabelSelector
				}
				return client.CoreV1().Pods(namespace).Watch(context.TODO(), lo)
			},
		},
		&v1.Pod{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				log.Debugf("Pod created: %s", pod.ObjectMeta.Name)
				applyTemplate(client, namespace)
			},
			UpdateFunc: func(old interface{}, obj interface{}) {
				pod1 := old.(*v1.Pod)
				pod2 := obj.(*v1.Pod)

				if pod1.Status.PodIP != pod2.Status.PodIP {
					log.Debugf("Pod updated: %s", pod2.ObjectMeta.Name)
					applyTemplate(client, namespace)
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				log.Debugf("Pod deleted: %s", pod.ObjectMeta.Name)
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
				log.Debugf("event: %v", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Infof("modified file: %s", event.Name)
					applyTemplate(client, namespace)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.WithError(err).Errorf("error: %v", err)
			}
		}
	}()

	err = watcher.Add(TemplatePath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not watch file: %s", TemplatePath)
	}

	return watcher, nil
}

func getFromEnv(envVar string, defaultValue string) (res string) {
	res = strings.TrimSpace(os.Getenv(envVar))
	if res == "" {
		res = defaultValue
	}
	return
}

func main() {
	log.AddHook(ContextHook{})
	log.SetLevel(log.DebugLevel)

	log.Infof("Starting up...")

	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		// Support the deprecated variable
		dsn = os.Getenv("SENTRY_URL")
		if dsn != "" {
			log.Warnf("Using the deprecated SENTRY_URL variable. Please switch to SENTRY_DSN.")
		}
	}

	if dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			// Either set your DSN here or set the SENTRY_DSN environment variable.
			Dsn: dsn,
			// Enable printing of SDK debug messages.
			// Useful when getting started or trying to figure something out.
			Debug: false,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}

		// Flush buffered events before the program terminates.
		// Set the timeout to the maximum duration the program can afford to wait.
		defer sentry.Flush(10 * time.Second)
	} else {
		log.Debugf("Skipping sentry init")
	}

	var config *rest.Config
	var err error
	var client kubernetes.Interface

	log.Infof("Reading InClusterConfig")

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
	LabelSelector = getFromEnv(LabelSelectorEnvVar, DefaultLabelSelector)

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
		w.WriteHeader(http.StatusNoContent)
		return
	})
	err = http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		panic(errors.WithStack(err))
	}
}
