#!/bin/bash

gcloud iam list-testable-permissions \
    "//cloudresourcemanager.googleapis.com/projects/$PROJECT_ID" \
    --format="json" > permissions.json
