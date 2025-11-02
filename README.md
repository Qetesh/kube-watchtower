[![kube-watchtower docker image](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml/badge.svg)](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml)


# kube-watchtower

kube-watchtower is a Kubernetes-native image update monitor inspired by Watchtower.
It automatically tracks container image updates within your Kubernetes cluster and safely performs rolling updates when new images are detected.

⚠️ kube-watchtower is currently in beta and not recommended for production use.

### ✨ Features
- ✅ Automatically monitors container image updates in Deployments, DaemonSets, and StatefulSets
- ✅ Detects containers with imagePullPolicy: Always
- ✅ Supports all image tags (latest, stable, version tags, etc.)
- ✅ Accurate digest tracking — reads the currently running image digest directly from Pods
- ✅ Uses Docker Registry API to check for updates
- ✅ Safely performs Kubernetes rollouts when new digests are available
- ✅ Supports notifications via Shoutrrr
- ✅ Namespace denylist support
- ✅ Supports scheduled via CronJob

---

## 🚀 Getting Started

### Prerequisites
- A running Kubernetes cluster
- Proper RBAC permissions for Deployment, DaemonSet, StatefulSet, and Pod management

---

### ⚙️ Configuration

Environment Variables

| **Variable**       | **Description**                                  | **Default** | **Example**         |
| ------------------ | ------------------------------------------------ | ----------- | ------------------- |
| DISABLE_NAMESPACES | Comma-separated list of excluded namespaces      | ""          | kube-system,default |
| NOTIFICATION_URL   | Notification URL (Shoutrrr format)               | ""          | See below           |
| NOTIFICATION_CLUSTER | Notification cluster name                      | kubernetes  | cluster1, cluster2  |
| LOG_LEVEL          | Log level (debug, info, warn, error)             | info        | debug, info         |

---

### 🔔 Notifications

kube-watchtower integrates with [Shoutrrr](https://containrrr.dev/shoutrrr/) to send notifications to various services.

---

### 🔍 Monitoring Rules

kube-watchtower monitors containers in Deployments, DaemonSets, and StatefulSets that meet all the following criteria:

- ✅ The container's imagePullPolicy is set to Always
- ✅ The namespace is not listed in DISABLE_NAMESPACES

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
- [x] Namespace denylist support
- [ ] Rollout timeout support
- [ ] Check only mode support
- [ ] Removes old images after updating(Due to architectural limits, image pruning is not supported. Suggestions are welcome)

---

### ❓ FAQ

Q: My container isn't being monitored. Why?

Ensure that imagePullPolicy is set to Always, and the namespace is not listed in DISABLE_NAMESPACES.

Q: Can I monitor private registries?

Yes. Make sure your cluster is configured with valid ImagePullSecrets.
kube-watchtower automatically uses the Pod's service account credentials.

Q: What happens if an update fails?

Kubernetes will automatically roll back the Deployment.
You can also receive failure notifications via your configured Shoutrrr channel.

Q: How do I exclude specific namespaces?

Set the DISABLE_NAMESPACES environment variable with a comma-separated list of namespace names to exclude.
Example: `DISABLE_NAMESPACES=kube-system,kube-public,default`

---

### 📜 License

Apache-2.0 license

---

### 💡 Acknowledgments
- Watchtower — inspiration
- Shoutrrr — notification framework
- The Kubernetes community

---

### 🤝 Contributing

Contributions, issues, and pull requests are welcome!
If you find a bug or have an idea for improvement, please open an issue.