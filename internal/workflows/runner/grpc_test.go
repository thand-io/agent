package runner

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// MockGreeterService implements a simple gRPC service for testing
type MockGreeterService struct {
	responses map[string]string
}

func NewMockGreeterService() *MockGreeterService {
	return &MockGreeterService{
		responses: make(map[string]string),
	}
}

func (m *MockGreeterService) SayHello(ctx context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	// Get the name field from the request using protoreflect API
	reqReflect := req.ProtoReflect()
	fds := reqReflect.Descriptor().Fields()
	nameField := fds.ByName("name")

	var name string
	if nameField != nil && reqReflect.Has(nameField) {
		name = reqReflect.Get(nameField).String()
	} else {
		name = "Unknown"
	}

	// Create response message using protoreflect
	reqDesc := req.ProtoReflect().Descriptor()
	fileDesc := reqDesc.ParentFile()
	respDesc := fileDesc.Messages().ByName("HelloResponse")
	if respDesc == nil {
		return nil, status.Error(codes.Internal, "response message type not found")
	}

	resp := dynamicpb.NewMessage(respDesc)
	respReflect := resp.ProtoReflect()
	msgField := respDesc.Fields().ByName("message")
	if msgField != nil {
		respReflect.Set(msgField, protoreflect.ValueOfString(fmt.Sprintf("Hello, %s!", name)))
	}

	return resp, nil
}

// setupMockGRPCServer creates a mock gRPC server for testing
func setupMockGRPCServer(t *testing.T) (string, func()) {
	// Create a listener on a random port
	lis, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)

	// Create gRPC server
	server := grpc.NewServer()

	// Register reflection service
	reflection.Register(server)

	// Start server in goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			// Server stopped, this is expected during cleanup
		}
	}()

	// Return address and cleanup function
	cleanup := func() {
		server.Stop()
		lis.Close()
	}

	return lis.Addr().String(), cleanup
}

func TestExecuteGRPCFunction_ConnectionFailure(t *testing.T) {
	// Create a workflow runner
	runner := &ResumableWorkflowRunner{
		workflowTask: &models.WorkflowTask{},
	}

	// Create a gRPC call with invalid host
	grpcCall := &model.CallGRPC{
		Call: "grpc",
		With: model.GRPCArguments{
			Proto: &model.ExternalResource{
				Endpoint: &model.Endpoint{
					URITemplate: &model.LiteralUri{Value: "file://test.proto"},
				},
			},
			Service: model.GRPCService{
				Name: "test.GreeterService",
				Host: "invalid-host",
				Port: 12345,
			},
			Method: "SayHello",
			Arguments: map[string]any{
				"name": "Test User",
			},
		},
	}

	// Execute the function and expect connection failure
	result, err := runner.executeGRPCFunction("test-task", grpcCall, nil)

	// Verify that we get a connection error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve service")
	assert.Nil(t, result)
}

func TestCreateGRPCConnection_Success(t *testing.T) {
	// Setup mock server
	addr, cleanup := setupMockGRPCServer(t)
	defer cleanup()

	// Parse host and port from address
	host, port, err := net.SplitHostPort(addr)
	assert.NoError(t, err)

	// Convert port to int
	var portInt int
	_, err = fmt.Sscanf(port, "%d", &portInt)
	assert.NoError(t, err)

	service := model.GRPCService{
		Name: "test.Service",
		Host: host,
		Port: portInt,
	}

	// Test connection creation
	conn, err := createGRPCConnection(service)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// Cleanup
	conn.Close()
}

func TestCreateGRPCConnection_Timeout(t *testing.T) {

	service := model.GRPCService{
		Name: "test.Service",
		Host: "10.255.255.1", // Non-routable IP to force timeout
		Port: 12345,
	}

	// Test connection creation (gRPC connections are lazy, so this will succeed)
	conn, err := createGRPCConnection(service)
	// Connection creation doesn't fail immediately with gRPC
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// Cleanup
	if conn != nil {
		conn.Close()
	}
}

func TestCreateGRPCContext_NoAuth(t *testing.T) {

	ctx, err := createGRPCContext(nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Verify no metadata is set
	md, ok := metadata.FromOutgoingContext(ctx)
	assert.True(t, !ok || len(md) == 0)
}

func TestCreateGRPCContext_BearerAuth(t *testing.T) {

	// Create bearer auth policy
	auth := &model.ReferenceableAuthenticationPolicy{
		AuthenticationPolicy: &model.AuthenticationPolicy{
			Bearer: &model.BearerAuthenticationPolicy{
				Token: "test-token",
			},
		},
	}

	ctx, err := createGRPCContext(nil, auth)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Verify bearer token is set in metadata
	md, ok := metadata.FromOutgoingContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, []string{"Bearer test-token"}, md["authorization"])
}

func TestCreateGRPCContext_BasicAuth(t *testing.T) {

	// Create basic auth policy
	auth := &model.ReferenceableAuthenticationPolicy{
		AuthenticationPolicy: &model.AuthenticationPolicy{
			Basic: &model.BasicAuthenticationPolicy{
				Username: "user",
				Password: "pass",
			},
		},
	}

	// This should not fail but should log a warning
	ctx, err := createGRPCContext(auth, nil)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Verify no metadata is set (basic auth not supported for gRPC)
	md, ok := metadata.FromOutgoingContext(ctx)
	assert.True(t, !ok || len(md) == 0)
}

func TestBuildRequestMessage_EmptyArguments(t *testing.T) {

	// Create a mock method descriptor using modern API
	mockMethodDesc := createMockProtoMethodDescriptor(t)

	// Test with empty arguments
	msg, err := buildRequestMessage(mockMethodDesc, map[string]any{})
	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestBuildRequestMessage_WithArguments(t *testing.T) {

	// Create a mock method descriptor using modern API
	mockMethodDesc := createMockProtoMethodDescriptor(t)

	// Test with arguments
	arguments := map[string]any{
		"name": "Test User",
	}

	msg, err := buildRequestMessage(mockMethodDesc, arguments)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
}

func TestConvertResponseToMap_NilMessage(t *testing.T) {

	result, err := convertResponseToMap(nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestConvertResponseToMap_ValidMessage(t *testing.T) {

	// Create a mock response message using modern API
	responseDesc := createMockProtoMessageDescriptor(t, "HelloResponse")
	msg := dynamicpb.NewMessage(responseDesc)

	// Set field using protoreflect API
	msgReflect := msg.ProtoReflect()
	msgField := responseDesc.Fields().ByName("message")
	if msgField != nil {
		msgReflect.Set(msgField, protoreflect.ValueOfString("Hello, World!"))
	}

	result, err := convertResponseToMap(msg)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Hello, World!", result["message"])
}

// Test workflow spec compliance
func TestGRPCCall_WorkflowSpecCompliance(t *testing.T) {
	// Test that our implementation matches the serverless workflow spec structure

	// Create a gRPC call that matches the spec example
	grpcCall := &model.CallGRPC{
		Call: "grpc",
		With: model.GRPCArguments{
			Proto: &model.ExternalResource{
				Endpoint: &model.Endpoint{
					URITemplate: &model.LiteralUri{Value: "file://app/greet.proto"},
				},
			},
			Service: model.GRPCService{
				Name: "GreeterApi.Greeter",
				Host: "localhost",
				Port: 5011,
			},
			Method: "SayHello",
			Arguments: map[string]any{
				"name": "${ .user.preferredDisplayName }",
			},
		},
	}

	// Verify all required fields are present
	assert.Equal(t, "grpc", grpcCall.Call)
	assert.NotNil(t, grpcCall.With.Proto)
	assert.Equal(t, "GreeterApi.Greeter", grpcCall.With.Service.Name)
	assert.Equal(t, "localhost", grpcCall.With.Service.Host)
	assert.Equal(t, 5011, grpcCall.With.Service.Port)
	assert.Equal(t, "SayHello", grpcCall.With.Method)
	assert.NotNil(t, grpcCall.With.Arguments)
	assert.Equal(t, "${ .user.preferredDisplayName }", grpcCall.With.Arguments["name"])
}

// createMockProtoMessageDescriptor creates a modern protoreflect.MessageDescriptor for testing
func createMockProtoMessageDescriptor(t *testing.T, messageName string) protoreflect.MessageDescriptor {
	// Create a simple message descriptor for testing using modern API
	fileDescProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String(messageName),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("message"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
		},
	}

	// Create file descriptor using modern API
	fileDesc, err := protodesc.NewFile(fileDescProto, protoregistry.GlobalFiles)
	assert.NoError(t, err)

	// Get message descriptor
	messages := fileDesc.Messages()
	assert.Equal(t, 1, messages.Len())
	messageDesc := messages.Get(0)

	return messageDesc
}

// createMockProtoMethodDescriptor creates a modern protoreflect.MethodDescriptor for testing
func createMockProtoMethodDescriptor(t *testing.T) protoreflect.MethodDescriptor {
	// Create a simple proto file descriptor for testing using modern API
	fileDescProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("HelloRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
			{
				Name: proto.String("HelloResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("message"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("GreeterService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("SayHello"),
						InputType:  proto.String(".test.HelloRequest"),
						OutputType: proto.String(".test.HelloResponse"),
					},
				},
			},
		},
	}

	// Create file descriptor using modern API
	fileDesc, err := protodesc.NewFile(fileDescProto, protoregistry.GlobalFiles)
	assert.NoError(t, err)

	// Get service descriptor
	services := fileDesc.Services()
	assert.Equal(t, 1, services.Len())
	serviceDesc := services.Get(0)

	// Get method descriptor
	methods := serviceDesc.Methods()
	assert.Equal(t, 1, methods.Len())
	methodDesc := methods.Get(0)

	return methodDesc
}

// Benchmark tests for performance validation
func BenchmarkCreateGRPCConnection(b *testing.B) {
	service := model.GRPCService{
		Name: "test.Service",
		Host: "localhost",
		Port: 12345,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, _ := createGRPCConnection(service)
		if conn != nil {
			conn.Close()
		}
	}
}

func BenchmarkCreateGRPCContext(b *testing.B) {
	auth := &model.ReferenceableAuthenticationPolicy{
		AuthenticationPolicy: &model.AuthenticationPolicy{
			Bearer: &model.BearerAuthenticationPolicy{
				Token: "test-token",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		createGRPCContext(nil, auth)
	}
}
