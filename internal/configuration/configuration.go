package configuration

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mailgun/mailgun-go/v4"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	configurationLog = ctrl.Log.WithName("configuration")
)

type Data struct {
	OperatorNamespace    string
	APIToken             string `json:"api_token,omitempty"`
	APIServer            string `json:"api_server,omitempty"`
	DomainVerifyDuration int    `json:"domain_verification_timeout,omitempty"`
}

func NewConfiguration() *Data {
	return &Data{
		OperatorNamespace:    "mailgun-operator-system",
		DomainVerifyDuration: 300, // 5 minutes
		APIToken:             "",
		APIServer:            mailgun.APIBaseEU,
	}
}

func (d *Data) LoadConfiguration(ctx context.Context, client client.Client, configMapName, secretName string) error {
	configData := make(map[string]string)

	// Load ConfigMap data if provided
	if configMapName != "" {
		configMapData, err := readConfigMap(ctx, client, d.OperatorNamespace, configMapName)
		if err != nil {
			return err
		}
		for k, v := range configMapData {
			configData[k] = v
		}
	}

	// Load Secret data if provided
	if secretName != "" {
		secretData, err := readSecret(ctx, client, d.OperatorNamespace, secretName)
		if err != nil {
			return err
		}
		for k, v := range secretData {
			configData[k] = string(v)
		}
	}

	if len(configData) > 0 {
		hashToConfig(d, configData)
	}

	return nil
}

func (d *Data) MailgunClient(domainName string) *mailgun.MailgunImpl {
	mg := mailgun.NewMailgun(domainName, d.APIToken)

	switch d.APIServer {
	case "EU":
		mg.SetAPIBase(mailgun.APIBaseEU)
	case "US":
		mg.SetAPIBase(mailgun.APIBaseUS)
	default:
		mg.SetAPIBase(d.APIServer)
	}

	return mg
}

func readConfigMap(ctx context.Context, c client.Client, namespace, configMapName string) (map[string]string, error) {
	configurationLog.Info("Loading configuration from ConfigMap",
		"name", configMapName,
		"namespace", namespace)
	configMap := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: configMapName}, configMap)
	if apierrs.IsNotFound(err) {
		configurationLog.Info("ConfigMap not found", "name", configMapName, "namespace", namespace)
		return nil, nil
	}
	if err != nil {
		configurationLog.Error(err, "Unable to read ConfigMap", "name", configMapName, "namespace", namespace)
		return nil, err
	}

	return configMap.Data, nil
}

func readSecret(ctx context.Context, c client.Client, namespace, secretName string) (map[string][]byte, error) {
	configurationLog.Info("Loading configuration from Secret", "name", secretName, "namespace", namespace)
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if apierrs.IsNotFound(err) {
		configurationLog.Info("Secret not found", "name", secretName, "namespace", namespace)
		return nil, nil
	}
	if err != nil {
		configurationLog.Error(err, "Unable to read Secret", "name", secretName, "namespace", namespace)
		return nil, err
	}
	return secret.Data, nil
}

func hashToConfig(d *Data, configData map[string]string) {
	count := reflect.TypeOf(d).Elem().NumField()
	for i := 0; i < count; i++ {
		field := reflect.TypeOf(d).Elem().Field(i)
		key := field.Tag.Get("json")

		if key == "" {
			continue
		} else {
			key = strings.Split(key, ",")[0]
		}

		if _, ok := configData[key]; !ok {
			continue
		}

		value := configData[key]

		switch t := field.Type; t.Kind() {
		case reflect.String:
			reflect.ValueOf(d).Elem().FieldByName(field.Name).SetString(value)
		case reflect.Int:
			intValue, err := strconv.ParseInt(value, 10, 0)
			if err != nil {
				errMsg := fmt.Sprintf("field %s, type %s, kind %s is not parsed",
					field.Name,
					t.String(),
					t.Kind())
				panic(errMsg)
			}
			reflect.ValueOf(d).Elem().FieldByName(field.Name).SetInt(intValue)
		}
	}
}
