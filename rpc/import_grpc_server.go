package rpc

import (
	"fmt"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/sentinel-sdk"
	"github.com/hashicorp/sentinel-sdk/encoding"
	"github.com/hashicorp/sentinel-sdk/proto/go"
	"golang.org/x/net/context"
)

// ImportGRPCServer is a gRPC server for Imports.
type ImportGRPCServer struct {
	F func() sdk.Import

	// instanceId is the current instance ID. This should be modified
	// with sync/atomic.
	instanceId    uint64
	instances     map[uint64]sdk.Import
	instancesLock sync.RWMutex
}

func (m *ImportGRPCServer) Close(
	ctx context.Context, v *proto.Close_Request) (*proto.Empty, error) {
	// Get the import and remove it immediately
	m.instancesLock.Lock()
	impt, ok := m.instances[v.InstanceId]
	delete(m.instances, v.InstanceId)
	m.instancesLock.Unlock()

	// If we have it, attempt to call Close on the import if it is
	// a closer.
	if ok {
		if c, ok := impt.(io.Closer); ok {
			c.Close()
		}
	}

	return &proto.Empty{}, nil
}

func (m *ImportGRPCServer) Configure(
	ctx context.Context, v *proto.Configure_Request) (*proto.Configure_Response, error) {
	// Build the configuration
	var config map[string]interface{}
	configRaw, err := encoding.ValueToGo(v.Config, reflect.TypeOf(config))
	if err != nil {
		return nil, fmt.Errorf("error converting config: %s", err)
	}
	config = configRaw.(map[string]interface{})

	// Configure is called once to configure a new import. Allocate the import.
	impt := m.F()

	// Call configure
	if err := impt.Configure(config); err != nil {
		return nil, err
	}

	// We have to allocate a new instance ID.
	id := atomic.AddUint64(&m.instanceId, 1)

	// Put the import into the store
	m.instancesLock.Lock()
	if m.instances == nil {
		m.instances = make(map[uint64]sdk.Import)
	}
	m.instances[id] = impt
	m.instancesLock.Unlock()

	// Configure the import
	return &proto.Configure_Response{
		InstanceId: id,
	}, nil
}

func (m *ImportGRPCServer) Get(
	ctx context.Context, v *proto.Get_MultiRequest) (*proto.Get_MultiResponse, error) {
	// Build the mapping of requests by instance ID. Then we can make the
	// calls for each proper instance easily.
	requestsById := make(map[uint64][]*sdk.GetReq)
	for _, req := range v.Requests {
		keys := make([]*sdk.GetKey, len(req.Keys))
		for i, reqKey := range req.Keys {
			keys[i] = &sdk.GetKey{Key: reqKey.Key}
			if reqKey.Args != nil {
				keys[i].Args = make([]interface{}, len(reqKey.Args))
				for j, arg := range reqKey.Args {
					obj, err := encoding.ValueToGo(arg, nil)
					if err != nil {
						return nil, fmt.Errorf("error converting arg %d: %s", i, err)
					}

					keys[i].Args[j] = obj
				}
			}
		}

		getReq := &sdk.GetReq{
			ExecId:       req.ExecId,
			ExecDeadline: time.Unix(int64(req.ExecDeadline), 0),
			Keys:         keys,
			KeyId:        req.KeyId,
		}

		requestsById[req.InstanceId] = append(requestsById[req.InstanceId], getReq)
	}

	responses := make([]*proto.Get_Response, 0, len(v.Requests))
	for id, reqs := range requestsById {
		m.instancesLock.RLock()
		impt, ok := m.instances[id]
		m.instancesLock.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown instance ID given: %d", id)
		}

		results, err := impt.Get(reqs)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			v, err := encoding.GoToValue(result.Value)
			if err != nil {
				return nil, err
			}

			responses = append(responses, &proto.Get_Response{
				InstanceId: id,
				KeyId:      result.KeyId,
				Keys:       result.Keys,
				Value:      v,
			})
		}
	}

	return &proto.Get_MultiResponse{Responses: responses}, nil
}
