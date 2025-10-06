package runner

import (
	"context"
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (r *ResumableWorkflowRunner) executeGRPCFunction(
	taskName string,
	call *model.CallGRPC,
	input any,
) (any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"call": call.Call,
	}).Info("Executing gRPC function call")

	grpcCall := call.With

	workflowTask := r.GetWorkflowTask()

	returnedInput, err := workflowTask.TraverseAndEvaluate(grpcCall.Arguments, input)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"task":  taskName,
			"input": input,
		}).WithError(err).Error("Failed to evaluate gRPC call arguments")

		return nil, fmt.Errorf("failed to evaluate gRPC call arguments: %w", err)
	}

	finalInput, ok := returnedInput.(map[string]any)

	if !ok && returnedInput != nil {
		return nil, fmt.Errorf("gRPC call arguments must evaluate to a map/object")
	}

	if workflowTask.HasTemporalContext() {

		// Execute as Temporal activity
		fut := workflow.ExecuteActivity(
			workflowTask.GetTemporalContext(),
			models.TemporalGrpcActivityName,
			grpcCall,
			finalInput,
		)

		var result any
		err := fut.Get(workflowTask.GetTemporalContext(), &result)
		if err != nil {
			return nil, fmt.Errorf("gRPC activity failed: %w", err)
		}

		return result, nil

	} else {

		return MakeGrpcRequest(grpcCall, finalInput)
	}

}

func MakeGrpcRequest(grpcCall model.GRPCArguments, finalInput map[string]any) (any, error) {

	// Step 1: Create gRPC connection
	conn, err := createGRPCConnection(grpcCall.Service)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}
	defer conn.Close()

	// Step 2: Create context with authentication
	ctx, err := createGRPCContext(grpcCall.Service.Authentication, grpcCall.Authentication)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC context: %w", err)
	}

	// Step 3: Use gRPC reflection to discover the service
	serviceDesc, methodDesc, err := resolveGRPCServiceAndMethod(ctx, conn, grpcCall.Service.Name, grpcCall.Method)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service and method: %w", err)
	}

	// Step 4: Build the request message dynamically
	reqMsg, err := buildRequestMessage(methodDesc, finalInput)
	if err != nil {
		return nil, fmt.Errorf("failed to build request message: %w", err)
	}

	// Step 5: Invoke the gRPC method
	respMsg, err := invokeGRPCMethod(ctx, conn, serviceDesc, methodDesc, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke gRPC method: %w", err)
	}

	// Step 6: Convert response to map
	result, err := convertResponseToMap(respMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert response: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"service": grpcCall.Service.Name,
		"method":  grpcCall.Method,
		"host":    grpcCall.Service.Host,
		"port":    grpcCall.Service.Port,
	}).Info("gRPC call completed successfully")

	return result, nil
}

// createGRPCConnection establishes a connection to the gRPC service
func createGRPCConnection(service model.GRPCService) (*grpc.ClientConn, error) {
	address := fmt.Sprintf("%s:%d", service.Host, service.Port)

	// Use insecure connection (can be enhanced to support TLS)
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC service at %s: %w", address, err)
	}

	return conn, nil
}

// createGRPCContext creates a context with authentication headers
func createGRPCContext(serviceAuth, callAuth *model.ReferenceableAuthenticationPolicy) (context.Context, error) {
	ctx := context.Background()

	// Prefer call-level authentication over service-level
	auth := callAuth
	if auth == nil {
		auth = serviceAuth
	}

	if auth == nil {
		return ctx, nil
	}

	md := metadata.New(nil)

	// Handle different authentication types
	if auth.AuthenticationPolicy.Basic != nil {
		// Basic auth would typically be handled at the HTTP/2 transport level
		// For gRPC, we might need to pass it as metadata
		logrus.Warn("Basic authentication for gRPC is not fully supported, consider using Bearer tokens")
	} else if auth.AuthenticationPolicy.Bearer != nil {
		md.Set("authorization", "Bearer "+auth.AuthenticationPolicy.Bearer.Token)
	}
	// Note: Other auth types can be added as needed

	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	return ctx, nil
}

// resolveGRPCServiceAndMethod uses modern gRPC reflection to resolve service and method descriptors
func resolveGRPCServiceAndMethod(ctx context.Context, conn *grpc.ClientConn, serviceName, methodName string) (protoreflect.ServiceDescriptor, protoreflect.MethodDescriptor, error) {
	// Get file descriptors from reflection
	fileDescProtos, err := getFileDescriptorsFromReflection(ctx, conn, serviceName)
	if err != nil {
		return nil, nil, err
	}

	// Parse descriptors and find service/method
	serviceDesc, methodDesc, err := findServiceAndMethod(fileDescProtos, serviceName, methodName)
	if err != nil {
		return nil, nil, err
	}

	return serviceDesc, methodDesc, nil
}

// getFileDescriptorsFromReflection retrieves file descriptors using gRPC reflection
func getFileDescriptorsFromReflection(ctx context.Context, conn *grpc.ClientConn, serviceName string) ([][]byte, error) {
	// Create reflection client
	refClient := grpc_reflection_v1.NewServerReflectionClient(conn)

	// Start reflection stream
	stream, err := refClient.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create reflection stream: %w", err)
	}
	defer stream.CloseSend()

	// Request file descriptor for the service
	req := &grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: serviceName,
		},
	}

	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("failed to send reflection request: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive reflection response: %w", err)
	}

	// Check for error response
	if errorResp := resp.GetErrorResponse(); errorResp != nil {
		return nil, fmt.Errorf("reflection error: %s", errorResp.GetErrorMessage())
	}

	// Get file descriptors
	fileDescResp := resp.GetFileDescriptorResponse()
	if fileDescResp == nil {
		return nil, fmt.Errorf("no file descriptor response")
	}

	return fileDescResp.GetFileDescriptorProto(), nil
}

// findServiceAndMethod parses file descriptors to find the specified service and method
func findServiceAndMethod(fileDescProtos [][]byte, serviceName, methodName string) (protoreflect.ServiceDescriptor, protoreflect.MethodDescriptor, error) {
	for _, fdBytes := range fileDescProtos {
		serviceDesc, methodDesc, found, err := parseFileDescriptor(fdBytes, serviceName, methodName)
		if err != nil {
			continue // Skip files that can't be parsed
		}
		if found {
			return serviceDesc, methodDesc, nil
		}
	}

	return nil, nil, fmt.Errorf("service %s or method %s not found", serviceName, methodName)
}

// parseFileDescriptor parses a single file descriptor and searches for service/method
func parseFileDescriptor(fdBytes []byte, serviceName, methodName string) (protoreflect.ServiceDescriptor, protoreflect.MethodDescriptor, bool, error) {
	fdProto := &descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(fdBytes, fdProto); err != nil {
		return nil, nil, false, err
	}

	fd, err := protodesc.NewFile(fdProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, nil, false, err
	}

	// Look for the service in this file
	serviceDesc, methodDesc := findServiceAndMethodInFile(fd, serviceName, methodName)
	if serviceDesc != nil && methodDesc != nil {
		return serviceDesc, methodDesc, true, nil
	}

	return nil, nil, false, nil
}

// findServiceAndMethodInFile searches for service and method within a file descriptor
func findServiceAndMethodInFile(fd protoreflect.FileDescriptor, serviceName, methodName string) (protoreflect.ServiceDescriptor, protoreflect.MethodDescriptor) {
	services := fd.Services()
	for i := 0; i < services.Len(); i++ {
		svc := services.Get(i)
		if string(svc.FullName()) == serviceName {
			// Found the service, now look for the method
			methodDesc := findMethodInService(svc, methodName)
			if methodDesc != nil {
				return svc, methodDesc
			}
			// Service found but method not found
			return svc, nil
		}
	}
	return nil, nil
}

// findMethodInService searches for a method within a service descriptor
func findMethodInService(svc protoreflect.ServiceDescriptor, methodName string) protoreflect.MethodDescriptor {
	methods := svc.Methods()
	for j := 0; j < methods.Len(); j++ {
		method := methods.Get(j)
		if string(method.Name()) == methodName {
			return method
		}
	}
	return nil
}

// buildRequestMessage builds a dynamic protobuf message from arguments
func buildRequestMessage(methodDesc protoreflect.MethodDescriptor, arguments map[string]any) (*dynamicpb.Message, error) {
	inputMsgDesc := methodDesc.Input()
	msg := dynamicpb.NewMessage(inputMsgDesc)

	if len(arguments) == 0 {
		return msg, nil
	}

	// Process each argument
	for fieldName, value := range arguments {
		// Find field using protoreflect
		protoFieldDesc := inputMsgDesc.Fields().ByName(protoreflect.Name(fieldName))
		if protoFieldDesc == nil {
			logrus.WithFields(logrus.Fields{
				"field":   fieldName,
				"message": string(inputMsgDesc.FullName()),
			}).Warn("Field not found in proto message, skipping")
			continue
		}

		// Set the field value using automatic conversion
		msg.Set(protoFieldDesc, protoreflect.ValueOf(value))
	}

	return msg, nil
}

// invokeGRPCMethod invokes a gRPC method using dynamic invocation
func invokeGRPCMethod(ctx context.Context, conn *grpc.ClientConn, serviceDesc protoreflect.ServiceDescriptor, methodDesc protoreflect.MethodDescriptor, reqMsg *dynamicpb.Message) (*dynamicpb.Message, error) {
	// Get the method's full name
	methodName := fmt.Sprintf("/%s/%s", serviceDesc.FullName(), methodDesc.Name())

	// Create response message
	outputMsgDesc := methodDesc.Output()
	respMsg := dynamicpb.NewMessage(outputMsgDesc)

	// Invoke the method using standard gRPC
	err := conn.Invoke(ctx, methodName, reqMsg, respMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke gRPC method: %w", err)
	}

	return respMsg, nil
}

// convertResponseToMap converts a dynamic protobuf message to a map
func convertResponseToMap(msg *dynamicpb.Message) (map[string]any, error) {
	if msg == nil {
		return nil, nil
	}

	// Use protoreflect to iterate over fields and convert to map
	result := make(map[string]any)
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fieldName := string(fd.Name())
		result[fieldName] = v.Interface() // Direct conversion using built-in method
		return true
	})

	return result, nil
}
