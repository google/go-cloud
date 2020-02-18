package mqttpubsub

import (
	"context"
	"errors"
	"gocloud.dev/gcerrors"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	defaultQOS byte = 1
	defaultCliendID = "go-cloud"
)

type mqttConn mqtt.Client

var (
	errInvalidMessage = errors.New("mqttpubsub: invalid or empty message")
	errConnRequired = errors.New("mqttpubsub: mqtt connection is required")
	errNotInitialized = errors.New("mqttpubsub: topic not initialized")
	errStillConnected = errors.New("mqttpubsub:  still connected. Kill all processes manually")
	errMQTTDisconnected = errors.New("mqttpubsub: disconnected")
)

func defaultMQTTConnect(url string) (cli mqtt.Client, err error) {
	opts := mqtt.NewClientOptions()
	opts = opts.AddBroker(url)
	opts.ClientID = defaultCliendID
	mqttClient := mqtt.NewClient(opts)

	token := mqttClient.Connect()
	token.Wait()
	if token.Error() != nil {
		return nil, token.Error()
	}
	if !mqttClient.IsConnectionOpen() {
		return nil, errMQTTDisconnected
	}

	return mqttClient, nil
}


func whichError(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized:
		return gcerrors.NotFound
	case mqtt.ErrInvalidTopicEmptyString, mqtt.ErrInvalidQos, mqtt.ErrInvalidTopicMultilevel:
		return gcerrors.FailedPrecondition
	}
	return gcerrors.Unknown
}