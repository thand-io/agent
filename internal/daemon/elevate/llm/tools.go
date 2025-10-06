package llm

import "google.golang.org/genai"

var GetUserRequest = "getUserRequest"
var GetWorkflowListToolName = "getWorkflowList"
var GetProviderListToolName = "getProviderList"
var EvaluateRequestToolName = "evaluateRequest"
var QueryElevationInfoToolName = "queryElevationInfo"
var GenerateRoleToolName = "generateRole"

func getToolchain(providers []string) []*genai.Tool {

	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				getUserRequestTool(),
				getProviderListTool(),
				getWorkflowListTool(),
				getEvaluationTools(providers),
				getQueryTools(),
				getRoleTools(),
			},
		},
	}

	return tools

}

/*
STEP 1
*/
func getEvaluationTools(providers []string) *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        EvaluateRequestToolName,
		Description: "Creates a new evaluation",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Required: []string{
				"request",
				"provider",
				"workflow",
				"duration",
				"rationale",
				"success",
			},
			Properties: map[string]*genai.Schema{
				"request": {
					Type:        genai.TypeString,
					Title:       "Request Details",
					Description: "Detailed elaboration of the user's request including specific services, resources, and access patterns needed. Consider dependencies and typical workflows for the use case.",
					Example:     "User needs to debug EC2 instance connectivity issues in production environment. Requires read access to EC2 instances, security groups, VPC configuration, and CloudWatch logs. Also needs ability to temporarily modify security group rules for testing connectivity.",
				},
				"provider": {
					Type:        genai.TypeString,
					Title:       "Provider",
					Description: "The cloud provider that best matches the user's request based on mentioned services, resources, or explicit provider names",
					Enum:        providers,
					Example:     "aws-prod",
				},
				"workflow": {
					Type:        genai.TypeString,
					Title:       "Workflow",
					Description: "The workflow that best matches the user's request based on the task they need to accomplish",
					Example:     "ec2-debug",
				},
				"duration": {
					Type:        genai.TypeString,
					Title:       "Duration",
					Description: "The duration for which the role is needed. Formatted as ISO 8601 duration",
					Format:      "duration",
					Example:     "1h",
				},
				"rationale": {
					Type:        genai.TypeString,
					Title:       "Rationale",
					Description: "Detailed explanation of how you determined the provider, expanded the request, and selected the duration. Include your reasoning for any assumptions made.",
					Example:     "Selected aws-prod based on mention of EC2 and production environment. Expanded request to include VPC and CloudWatch access as these are typically needed for connectivity debugging. Chose 2h duration as sufficient for investigation and testing while minimizing security exposure.",
				},
				"success": {
					Type:        genai.TypeBoolean,
					Title:       "Request Success",
					Description: "Indicates whether the evaluation was successful",
					Example:     true,
				},
			},
		},
		Response: &genai.Schema{
			Type:        genai.TypeObject,
			Description: "The provider that best matches the user's request",
			Required: []string{
				"provider",
				"duration",
			},
			Properties: map[string]*genai.Schema{
				"provider": {
					Type:        genai.TypeObject,
					Description: "The cloud provider that best matches the user's request based on mentioned services, resources, or explicit provider names",
					Example: map[string]any{
						"name":        "aws-prod",
						"provider":    "aws",
						"description": "AWS Production Account",
						"enabled":     true,
					},
					Properties: map[string]*genai.Schema{
						"name": {
							Type:        genai.TypeString,
							Description: "The name of the provider that best matches the user's request",
							Example:     "aws-prod",
						},
						"provider": {
							Type:        genai.TypeString,
							Description: "The cloud provider that best matches the user's request based on mentioned services, resources, or explicit provider names",
							Enum:        []string{"aws", "gcp", "azure"},
							Example:     "aws",
						},
						"description": {
							Type:        genai.TypeString,
							Description: "A brief description of the provider",
							Example:     "AWS Production Account",
						},
						"enabled": {
							Type:        genai.TypeBoolean,
							Description: "Indicates if the provider is enabled",
							Example:     true,
						},
					},
				},
				"workflow": {
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name": {
							Type:        genai.TypeString,
							Description: "The name of the workflow that best matches the user's request",
							Example:     "ec2-debug",
						},
						"description": {
							Type:        genai.TypeString,
							Description: "A brief description of the workflow",
							Example:     "Workflow for debugging EC2 instances",
						},
					},
				},
				"request": {
					Type:        genai.TypeString,
					Title:       "Request Details",
					Description: "Detailed elaboration of the user's request including specific services, resources, and access patterns needed. Consider dependencies and typical workflows for the use case.",
					Example:     "User needs to debug EC2 instance connectivity issues in production environment. Requires read access to EC2 instances, security groups, VPC configuration, and CloudWatch logs. Also needs ability to temporarily modify security group rules for testing connectivity.",
				},
				"duration": {
					Type:        genai.TypeString,
					Title:       "Duration",
					Description: "The validated duration for which the role is needed. Formatted as ISO 8601 duration",
					Format:      "duration",
					Example:     "1h",
				},
			},
		},
	}
}

/*
STEP 2
*/
func getQueryTools() *genai.FunctionDeclaration {

	return &genai.FunctionDeclaration{
		Name:        QueryElevationInfoToolName,
		Description: "Queries existing roles, permissions and resources",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Required: []string{
				"rationale",
				"success",
			},
			Properties: map[string]*genai.Schema{
				"roles": {
					Type:        genai.TypeArray,
					Description: "Provide a list of search terms for roles that might be relevant to the request. These should be existing roles that provide a baseline of needed permissions. The final role can inherit from one or more of these roles to build upon established security patterns.",
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
					Example: []string{
						"EC2-ReadOnly",
						"Network-Debugger",
					},
				},
				"permissions": {
					Type:        genai.TypeArray,
					Description: "Provide a list of search terms for specific permissions required for the task. Use service:action format (e.g., ec2:DescribeInstances). These should be the minimum additional permissions needed beyond any inherited roles.",
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
					Example: []string{
						"ec2:DescribeInstances",
						"ec2:StartInstances",
					},
				},
				"resources": {
					Type:        genai.TypeArray,
					Description: "Provide a list of search terms for resources that are relevant to the request. These can be specific resource IDs, ARNs, or patterns. Use specific ARNs when possible, wildcards only when necessary with appropriate boundaries.",
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
					Example: []string{
						"i-0123456789abcdef0",
						"vpc-0abcdef1234567890",
						"sg-0abcdef1234567890",
						"us-west-2",
					},
				},
				"rationale": {
					Type:        genai.TypeString,
					Description: "Detailed explanation of how you identified the roles, permissions, and resources. Include your reasoning for any assumptions made.",
					Example:     "Identified 'EC2-ReadOnly' role for baseline instance visibility and 'Network-Debugger' for VPC and security group access. Selected specific permissions like ec2:AuthorizeSecurityGroupIngress for temporary rule changes and cloudwatch:GetMetricData for performance analysis. Resources focused on production account and specific VPC based on request context.",
				},
				"success": {
					Type:        genai.TypeBoolean,
					Title:       "Request Success",
					Description: "Indicates whether you were able to find relevant roles, permissions, and or resources",
					Example:     true,
				},
			},
		},
		Response: &genai.Schema{
			Type:        genai.TypeObject,
			Description: "The roles, permissions and resources that match the user's request",
			Properties: map[string]*genai.Schema{
				"roles": {
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
				},
				"permissions": {
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
				},
				"resources": {
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
				},
			},
		},
	}
}

/*
STEP 3
*/
func getRoleTools() *genai.FunctionDeclaration {

	return &genai.FunctionDeclaration{
		Name:        GenerateRoleToolName,
		Description: "Creates a new role. It is critical that you provide either a role to inherit from, explicit permissions or both.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Required: []string{
				"name",
				"description",
				"rationale",
				"success",
			},
			Properties: map[string]*genai.Schema{
				"name": {
					Type:        genai.TypeString,
					Description: "Descriptive, professional role name that indicates purpose and temporary nature. Use format: TaskType-Provider-TimeScope",
					Example:     "EC2-Debug-Temporary-2h",
				},
				"description": {
					Type:        genai.TypeString,
					Description: "Clear explanation of the role's purpose, scope, and time limitation",
					Example:     "Temporary role for debugging EC2 connectivity issues in production. Grants minimal required permissions for 2 hours.",
				},
				"inherits": {
					Type:        genai.TypeArray,
					Description: "List of existing roles to inherit permissions from. Inherited roles provide baseline permissions that can be supplemented with additional explicit permissions. All allow/deny rules from inherited roles are automatically included. Use this to build upon established security patterns.",
					Items: &genai.Schema{
						Type: genai.TypeString,
					},
					Example: []string{
						"EC2-ReadOnly",
						"Network-Debugger",
					},
				},
				"permissions": {
					Type:        genai.TypeObject,
					Description: "Explicit permissions for the role. Can be used alone or in combination with inherited roles. When used with inheritance, these permissions supplement the inherited ones.",
					Properties: map[string]*genai.Schema{
						"allow": {
							Type:        genai.TypeArray,
							Description: "Additional specific permissions required for the task beyond any inherited ones. Use service:action format (e.g., ec2:DescribeInstances). Prefer specific actions over wildcards. These permissions are added to any inherited allow permissions.",
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Example: []string{
								"ec2:DescribeInstances",
								"ec2:StartInstances",
							},
						},
						"deny": {
							Type:        genai.TypeArray,
							Description: "Explicitly denied permissions to prevent privilege escalation or access to sensitive resources. These denials override any inherited allow permissions and can restrict inherited roles.",
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Example: []string{
								"ec2:StopInstances",
								"ec2:TerminateInstances",
							},
						},
					},
				},
				"resources": {
					Type:        genai.TypeObject,
					Description: "Resource restrictions for the role",
					Properties: map[string]*genai.Schema{
						"allow": {
							Type:        genai.TypeArray,
							Description: "Resource ARNs or patterns the role can access. Use specific ARNs when possible, wildcards only when necessary with appropriate boundaries.",
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Example: []string{
								"arn:aws:ec2:us-east-1:123456789012:instance/i-0123456789abcdef0",
								"arn:aws:ec2:us-east-1:123456789012:instance/i-0abcdef1234567890",
							},
						},
						"deny": {
							Type:        genai.TypeArray,
							Description: "Resource ARNs or patterns explicitly denied to restrict access to sensitive resources",
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Example: []string{
								"arn:aws:ec2:us-east-1:123456789012:instance/i-0123456789abcdef0",
								"arn:aws:ec2:us-east-1:123456789012:instance/i-0abcdef1234567890",
							},
						},
					},
				},
				"rationale": {
					Type:        genai.TypeString,
					Description: "Detailed security analysis explaining role design choices including: inheritance decisions (which roles chosen and why), explicit permission choices, resource restrictions, potential risks, and how the role follows least privilege principles. If using inheritance, explain what the inherited roles provide and any additional permissions added.",
					Example:     "Inherited from 'EC2-ReadOnly' role for baseline instance visibility, then added ec2:AuthorizeSecurityGroupIngress for temporary rule changes and cloudwatch:GetMetricData for performance analysis. Resources restricted to prod account and specific VPC. Risk mitigation includes explicit deny on IAM actions and production database access.",
				},
				"success": {
					Type:        genai.TypeBoolean,
					Title:       "Request Success",
					Description: "Indicates whether you were able to create a suitable role",
					Example:     true,
				},
			},
		},
	}
}

/*
Suplementary function to add metadata to tool calls
*/

func getProviderListTool() *genai.FunctionDeclaration {

	return &genai.FunctionDeclaration{
		Name:        GetProviderListToolName,
		Description: "Returns a list of available providers",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"query": {
					Type:        genai.TypeString,
					Description: "Optional search query to filter providers by name or description",
					Example:     "aws",
				},
			},
		},
		Response: &genai.Schema{
			Type:  genai.TypeArray,
			Items: getProviderSchema(),
		},
	}
}

func getProviderSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"name": {
				Type:        genai.TypeString,
				Description: "The name of the provider",
				Example:     "aws-prod",
			},
			"provider": {
				Type:        genai.TypeString,
				Description: "The cloud provider type",
				Enum:        []string{"aws", "gcp", "azure"},
				Example:     "aws",
			},
			"description": {
				Type:        genai.TypeString,
				Description: "A brief description of the provider",
				Example:     "AWS Production Account",
			},
			"enabled": {
				Type:        genai.TypeBoolean,
				Description: "Indicates if the provider is enabled",
				Example:     true,
			},
		},
	}
}
func getWorkflowListTool() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        GetWorkflowListToolName,
		Description: "Returns a list of available workflows",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"query": {
					Type:        genai.TypeString,
					Description: "Optional search query to filter workflows by name or description",
					Example:     "debug",
				},
			},
		},
		Response: &genai.Schema{
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {
						Type:        genai.TypeString,
						Description: "The name of the workflow",
						Example:     "ec2-debug",
					},
					"description": {
						Type:        genai.TypeString,
						Description: "A brief description of the workflow",
						Example:     "Workflow for debugging EC2 instances",
					},
				},
			},
		},
	}
}

func getUserRequestTool() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        GetUserRequest,
		Description: "Extracts and expands the user's access request details",
		Parameters: &genai.Schema{
			Type:       genai.TypeObject,
			Properties: map[string]*genai.Schema{},
		},
		Response: &genai.Schema{
			Type:        genai.TypeObject,
			Description: "The expanded user request details",
			Required: []string{
				"reason",
			},
			Properties: map[string]*genai.Schema{
				"reason": {
					Type:        genai.TypeString,
					Description: "Detailed elaboration of the user's request including specific services, resources, and access patterns needed. Consider dependencies and typical workflows for the use case.",
					Example:     "User needs to debug EC2 instance connectivity issues in production environment. Requires read access to EC2 instances, security groups, VPC configuration, and CloudWatch logs. Also needs ability to temporarily modify security group rules for testing connectivity.",
				},
			},
		},
	}
}

func getLabels() map[string]string {
	return map[string]string{
		"request": "test",
	}
}
