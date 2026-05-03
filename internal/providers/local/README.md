
Server:

- Registers the local provider and capabilites depending on provider runtime
  for each capabilty
- Roles and permissions defined in config or via AD or IDP
- Tenants are the agent systems we want to gain access to

Agent:

- Registers with the server
- Sees the provider config and capabilities
- Registers those capabilities as local activities / workflows
- 


- re-factor the notifcation system to work like the auth/revoke sub-workflows
- then we can create our notification path for thand workflow tasks like the existing email and slack notifications
- 