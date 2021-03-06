package provisioning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/runtime/components"

	"github.com/Azure/azure-sdk-for-go/services/eventhub/mgmt/2017-04-01/eventhub"
	"github.com/sirupsen/logrus"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler/azure"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process"
	processazure "github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process/azure"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/ptr"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"
	"github.com/kyma-project/control-plane/components/provisioner/pkg/gqlschema"
)

const (
	authorizationRuleName = "RootManageSharedAccessKey"

	kafkaPort = "9093"

	k8sSecretNamespace = "knative-eventing"
	kafkaProvider      = "azure"

	// prefix is added before the created Azure resources
	// to satisfy Azure naming validation: https://docs.microsoft.com/en-us/rest/api/servicebus/create-namespace
	prefix = "k"
)

// ensure the interface is implemented
var _ Step = (*ProvisionAzureEventHubStep)(nil)

type ProvisionAzureEventHubStep struct {
	operationManager *process.ProvisionOperationManager
	processazure.EventHub
}

func NewProvisionAzureEventHubStep(os storage.Operations, hyperscalerProvider azure.HyperscalerProvider, accountProvider hyperscaler.AccountProvider, ctx context.Context) *ProvisionAzureEventHubStep {
	return &ProvisionAzureEventHubStep{
		operationManager: process.NewProvisionOperationManager(os),
		EventHub: processazure.EventHub{
			HyperscalerProvider: hyperscalerProvider,
			AccountProvider:     accountProvider,
			Context:             ctx,
		},
	}
}

func (p *ProvisionAzureEventHubStep) Name() string {
	return "Provision Azure Event Hubs"
}

func (p *ProvisionAzureEventHubStep) Run(operation internal.ProvisioningOperation, log logrus.FieldLogger) (internal.ProvisioningOperation, time.Duration, error) {
	hypType := hyperscaler.Azure
	log.Infof("HAP lookup for credentials to provision cluster for global account ID %s on Hyperscaler %s", operation.ProvisioningParameters.ErsContext.GlobalAccountID, hypType)

	// get hyperscaler credentials from HAP
	credentials, err := p.EventHub.AccountProvider.GardenerCredentials(hypType, operation.ProvisioningParameters.ErsContext.GlobalAccountID)
	if err != nil {
		// retrying might solve the issue, the HAP could be temporarily unavailable
		errorMessage := fmt.Sprintf("Unable to retrieve Gardener Credentials from HAP lookup: %v", err)
		return p.operationManager.RetryOperation(operation, errorMessage, time.Minute, time.Minute*30, log)
	}
	azureCfg, err := azure.GetConfigFromHAPCredentialsAndProvisioningParams(credentials, operation.ProvisioningParameters)
	if err != nil {
		// internal error, repeating doesn't solve the problem
		errorMessage := fmt.Sprintf("Failed to create Azure config: %v", err)
		return p.operationManager.OperationFailed(operation, errorMessage)
	}

	// create hyperscaler client
	azureClient, err := p.EventHub.HyperscalerProvider.GetClient(azureCfg, log)
	if err != nil {
		// internal error, repeating doesn't solve the problem
		errorMessage := fmt.Sprintf("Failed to create Azure EventHubs client: %v", err)
		return p.operationManager.OperationFailed(operation, errorMessage)
	}

	// prepare azure tags
	tags := azure.Tags{
		azure.TagSubAccountID: &operation.ProvisioningParameters.ErsContext.SubAccountID,
		azure.TagInstanceID:   &operation.InstanceID,
		azure.TagOperationID:  &operation.ID,
	}

	// prepare a valid unique name for Azure resources
	uniqueName := getAzureResourceName(operation.InstanceID)

	// create Resource Group
	groupName := uniqueName
	resourceGroup, err := azureClient.CreateResourceGroup(p.EventHub.Context, azureCfg, groupName, tags)
	if err != nil {
		// retrying might solve the issue while communicating with azure, e.g. network problems etc
		errorMessage := fmt.Sprintf("Failed to persist Azure Resource Group [%s] with error: %v", groupName, err)
		return p.operationManager.RetryOperation(operation, errorMessage, time.Minute, time.Minute*30, log)
	}
	log.Printf("Persisted Azure Resource Group [%s]", groupName)

	// create EventHubs Namespace
	eventHubsNamespace := uniqueName
	eventHubNamespace, err := azureClient.CreateNamespace(p.EventHub.Context, azureCfg, groupName, eventHubsNamespace, tags)
	if err != nil {
		// retrying might solve the issue while communicating with azure, e.g. network problems etc
		errorMessage := fmt.Sprintf("Failed to persist Azure EventHubs Namespace [%s] with error: %v", eventHubsNamespace, err)
		return p.operationManager.RetryOperation(operation, errorMessage, time.Minute, time.Minute*30, log)
	}
	log.Printf("Persisted Azure EventHubs Namespace [%s]", eventHubsNamespace)

	// get EventHubs Namespace secret
	accessKeys, err := azureClient.GetEventhubAccessKeys(p.EventHub.Context, *resourceGroup.Name, *eventHubNamespace.Name, authorizationRuleName)
	if err != nil {
		// retrying might solve the issue while communicating with azure, e.g. network problems etc
		errorMessage := fmt.Sprintf("Unable to retrieve access keys to azure event-hub namespace: %v", err)
		return p.operationManager.RetryOperation(operation, errorMessage, time.Minute, time.Minute*30, log)
	}
	if accessKeys.PrimaryConnectionString == nil {
		// if GetEventhubAccessKeys() does not fail then a non-nil accessKey is returned
		// then retry the operation once
		errorMessage := "PrimaryConnectionString is nil"
		return p.operationManager.RetryOperationOnce(operation, errorMessage, time.Second*15, log)
	}
	kafkaEndpoint := extractEndpoint(accessKeys)
	kafkaPassword := *accessKeys.PrimaryConnectionString

	// append installation overrides
	operation.InputCreator.AppendOverrides(components.KnativeEventing, getKnativeEventingOverrides())
	operation.InputCreator.AppendOverrides(components.KnativeEventingKafka, getKafkaChannelOverrides(kafkaEndpoint, kafkaPort, k8sSecretNamespace, "$ConnectionString", kafkaPassword, kafkaProvider))

	return operation, 0, nil
}

func extractEndpoint(accessKeys eventhub.AccessKeys) string {
	endpoint := strings.Split(*accessKeys.PrimaryConnectionString, ";")[0]
	endpoint = strings.TrimPrefix(endpoint, "Endpoint=sb://")
	endpoint = strings.TrimSuffix(endpoint, "/")
	return endpoint
}

func getKnativeEventingOverrides() []*gqlschema.ConfigEntryInput {
	return []*gqlschema.ConfigEntryInput{
		{
			Key:   "knative-eventing.channel.default.apiVersion",
			Value: "knativekafka.kyma-project.io/v1alpha1",
		},
		{
			Key:   "knative-eventing.channel.default.kind",
			Value: "KafkaChannel",
		},
	}
}

func getKafkaChannelOverrides(brokerHostname, brokerPort, namespace, username, password, kafkaProvider string) []*gqlschema.ConfigEntryInput {
	return []*gqlschema.ConfigEntryInput{
		{
			Key:    "kafka.brokers.hostname",
			Value:  brokerHostname,
			Secret: ptr.Bool(true),
		},
		{
			Key:    "kafka.brokers.port",
			Value:  brokerPort,
			Secret: ptr.Bool(true),
		},
		{
			Key:    "kafka.namespace",
			Value:  namespace,
			Secret: ptr.Bool(true),
		},
		{
			Key:    "kafka.password",
			Value:  password,
			Secret: ptr.Bool(true),
		},
		{
			Key:    "kafka.username",
			Value:  username,
			Secret: ptr.Bool(true),
		},
		{
			Key:    "kafka.secretName",
			Value:  "knative-kafka",
			Secret: ptr.Bool(true),
		},
		{
			Key:    "environment.kafkaProvider",
			Value:  kafkaProvider,
			Secret: ptr.Bool(true),
		},
	}
}

// getAzureResourceName returns a valid Azure resource name that is in lower case and starts with a letter.
func getAzureResourceName(name string) string {
	name = fmt.Sprintf("%s%s", prefix, name)
	name = strings.ToLower(name)
	return name
}
