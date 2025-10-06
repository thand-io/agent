This is an intelligent scheduling package for Go applications.

Depending on the platform provided the scheduler will use the appropriate scheduling mechanism.

Google functions -> Cloud Scheduler
Aws Lambda -> EventBridge
Local Binary -> gocron
Docker -> cron
Kubernetes -> kube-scheduler
