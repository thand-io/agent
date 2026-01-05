# List services
gcloud run services list --project=thand

# Cloud Run service with IAP requires a load balancer + backend service
# First, find your backend service name:
# gcloud compute backend-services list --project=thand

# Then set IAP settings on the backend service (not the Cloud Run service directly):
gcloud beta iap settings set iap_settings.yaml \
  --project=thand  \
  --resource-type=cloud-run \
  --service=agent \
  --region=europe-west1

# Note: Cloud Run service 'agent' in europe-west1 must be behind an HTTPS load balancer
# with a serverless NEG for IAP to work
