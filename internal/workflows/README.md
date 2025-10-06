The internal workflows make use of serverless workflows to execute the workflow definitions.

For elevations:

1. The user provides the role and resource they want to elevate.
    example: "elevate role admin on provider aws-prod"

2. The system then picks the workflow that matches the role and resource.
    example: "workflow: aws-prod-admin"

3. The workflow is then executed but not before the authentication flow is
executed. 

4. The rest of the workflow is then executed.

5. User is elevated to the role and resource.
