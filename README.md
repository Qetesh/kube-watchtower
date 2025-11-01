[![kube-watchtower docker image](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml/badge.svg)](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml)


# kube-watchtower

kube-watchtower is a Kubernetes-native image update monitor inspired by Watchtower.
It automatically tracks container image updates within your Kubernetes cluster and safely performs rolling updates when new images are detected.

⚠️ kube-watchtower is currently in beta and not recommended for production use.

### ✨ Features
	•	✅ Automatically monitors container image updates in Deployments, DaemonSets, and StatefulSets
	•	✅ Detects containers with imagePullPolicy: Always
	•	✅ Supports all image tags (latest, stable, version tags, etc.)
	•	✅ Accurate digest tracking — reads the currently running image digest directly from Pods
	•	✅ Uses Docker Registry API to check for updates
	•	✅ Safely performs Kubernetes rollouts when new digests are available
	•	✅ Supports notifications via Shoutrrr
	•	✅ Namespace denylist support
	•	✅ Supports scheduled via CronJob

---

## 🚀 Getting Started

### Prerequisites
	•	A running Kubernetes cluster
	•	Proper RBAC permissions for Deployment, DaemonSet, StatefulSet, and Pod management

---

### ⚙️ Configuration

Environment Variables

| **Variable**       | **Description**                                  | **Default** | **Example**         |
| ------------------ | ------------------------------------------------ | ----------- | ------------------- |
| NAMESPACE          | Namespace to monitor (empty = all)               | ""          | default, production |
| DISABLE_CONTAINERS | Comma-separated list of excluded container names | ""          | nginx,redis         |
| NOTIFICATION_URL   | Notification URL (Shoutrrr format)               | ""          | See below           |
| NOTIFICATIONS_CLUSTER   | Notification cluster name                        | ""          | cluster1, cluster2 |

---

### 🔔 Notifications

kube-watchtower integrates with [Shoutrrr](https://containrrr.dev/shoutrrr/) to send notifications to various services.

---

### 🔍 Monitoring Rules

kube-watchtower monitors containers in Deployments, DaemonSets, and StatefulSets that meet all the following criteria:

	1.	✅ The container's imagePullPolicy is set to Always
	2.	✅ The container is not listed in DISABLE_NAMESPACE

---

### 🆚 Comparison: Watchtower vs. kube-watchtower

| **Feature**        | **Watchtower** | **kube-watchtower** |
| ------------------ | -------------- | ------------------ |
| Runtime            | Docker         | Kubernetes         |
| Update Method      | Container restart | Kubernetes rollout |
| Configuration      | Container labels | Environment variables + RBAC |
| Image Check        | Docker API      | Docker Registry API |
| High Availability | Single instance | Managed by Kubernetes |


---

### Todo

- [x] Deployments, DaemonSet, StatefulSets
- [x] Notifier formatter(Start log, Update log)
- [x] CronJob support
- [x] Private registry support via ImagePullSecrets
- [ ] Namespace denylist support
- [ ] Rollout timeout support
- [ ] Check only mode support

---

### ❓ FAQ

Q: My container isn’t being monitored. Why?

Ensure that imagePullPolicy is set to Always, and the container name is not listed in DISABLE_CONTAINERS.

Q: Can I monitor private registries?

Yes. Make sure your cluster is configured with valid ImagePullSecrets.
kube-watchtower automatically uses the Pod’s service account credentials.

Q: What happens if an update fails?

Kubernetes will automatically roll back the Deployment.
You can also receive failure notifications via your configured Shoutrrr channel.

Q: Can I monitor multiple namespaces?

Yes. Leave the NAMESPACE variable empty to monitor all namespaces (requires proper RBAC permissions).

---

### 📜 License

Apache-2.0 license

---

### 💡 Acknowledgments
	•	Watchtower — inspiration
	•	Shoutrrr — notification framework
	•	The Kubernetes community

---

### 🤝 Contributing

Contributions, issues, and pull requests are welcome!
If you find a bug or have an idea for improvement, please open an issue.