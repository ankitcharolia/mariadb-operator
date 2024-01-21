package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	mdbhttp "github.com/mariadb-operator/mariadb-operator/pkg/http"
)

var ErrMasterServerNotFound = errors.New("master server not found")

type ServerParameters struct {
	Address  string    `json:"address"`
	Port     int32     `json:"port"`
	Protocol string    `json:"protocol"`
	Params   MapParams `json:"-"`
}

func (s ServerParameters) MarshalJSON() ([]byte, error) {
	type ServerParametersInternal ServerParameters // prevent recursion
	bytes, err := json.Marshal(ServerParametersInternal(s))
	if err != nil {
		return nil, err
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &rawMap); err != nil {
		return nil, err
	}

	for k, v := range s.Params {
		if _, ok := rawMap[k]; ok { // prevent overriding
			continue
		}
		bytes, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		rawMap[k] = bytes
	}

	return json.Marshal(rawMap)
}

type ServerAttributes struct {
	State      string           `json:"state,omitempty"`
	Parameters ServerParameters `json:"parameters"`
}

func (s *ServerAttributes) IsMaster() bool {
	return strings.Contains(s.State, "Master")
}

type ServerClient struct {
	GenericClient[ServerAttributes]
}

func NewServerClient(client *mdbhttp.Client) *ServerClient {
	return &ServerClient{
		GenericClient: NewGenericClient[ServerAttributes](
			client,
			"servers",
			ObjectTypeServers,
		),
	}
}

func (s *ServerClient) GetMaster(ctx context.Context) (string, error) {
	servers, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	for _, srv := range servers {
		if srv.Attributes.IsMaster() {
			return srv.ID, nil
		}
	}
	return "", ErrMasterServerNotFound
}
