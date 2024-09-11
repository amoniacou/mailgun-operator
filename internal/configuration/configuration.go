package configuration

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

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
	APIToken             string
	OperatorNamespace    string `json:"operator_namespace,omitempty"`
	APITokenSecret       string `json:"api_token,omitempty"`
	DomainVerifyDuration int    `json:"domain_verification_timeout,omitempty"`
}

func NewConfiguration() *Data {
	return &Data{
		OperatorNamespace: "mailgun-operator-system",
		APIToken:          "",
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
		count := reflect.TypeOf(configData).Elem().NumField()
		for i := 0; i < count; i++ {
			field := reflect.TypeOf(configData).Elem().Field(i)
			key := field.Tag.Get("json")
			if key == "" {
				continue
			}

			// Initialize value with default
			var value string

			valueField := reflect.ValueOf(configData).Elem().FieldByName(field.Name)
			switch valueField.Kind() {
			case reflect.Int:
				value = strconv.Itoa(int(valueField.Int()))

			case reflect.String:
				value = valueField.String()
			default:
				value = valueField.String()
			}

			switch t := field.Type; t.Kind() {
			case reflect.String:
				reflect.ValueOf(d).Elem().FieldByName(field.Name).SetString(value)
			case reflect.Int:
				intValue, err := strconv.ParseInt(value, 10, 0)
				if err != nil {
					configurationLog.Info(
						"Skipping configuration value due to invalid type",
						"field", field.Name, "value", value, "error", err)
				}
				reflect.ValueOf(d).Elem().FieldByName(field.Name).SetInt(intValue)
			default:
				errMsg := fmt.Sprintf("field %s, type %s, kind %s is not parsed",
					field.Name,
					t.String(),
					t.Kind())
				panic(errMsg)
			}

		}
	}

	return nil
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
