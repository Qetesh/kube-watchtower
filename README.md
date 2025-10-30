[![kube-watchtower docker image](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml/badge.svg)](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml)


# kube-watchtower

kube-watchtower is a Kubernetes-native image update monitor inspired by Watchtower.
It automatically tracks container image updates within your Kubernetes cluster and safely performs rolling updates when new images are detected.

⚠️ kube-watchtower is currently in beta and not recommended for production use.

### ✨ Features
	•	✅ Automatically monitors container image updates in Deployments, DaemonSets, and StatefulSets
	•	✅ Detects containers with imagePullPolicy: Always
	•	✅ Only updates images tagged as latest (prevents unwanted fixed-version updates)
	•	✅ Accurate digest tracking — reads the currently running image digest directly from Pods
	•	✅ Uses Docker Registry API to check for updates
	•	✅ Safely performs Kubernetes rollouts when new digests are available
	•	✅ Supports notifications via Shoutrrr
	•	✅ Container blacklist support
	•	✅ Automatically cleans up old resources (ReplicaSets for Deployments, ControllerRevisions for DaemonSets/StatefulSets)
	•	✅ Supports scheduled and continuous operation modes

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
| CHECK_INTERVAL     | Interval between update checks                   | 5m          | 10m, 1h             |
| NAMESPACE          | Namespace to monitor (empty = all)               | ""          | default, production |
| CLEANUP            | Automatically clean up old resources             | true        | true, false         |
| DISABLE_CONTAINERS | Comma-separated list of excluded container names | ""          | nginx,redis         |
| NOTIFICATION_URL   | Notification URL (Shoutrrr format)               | ""          | See below           |
| NOTIFICATIONS_CLUSTER   | Notification cluster name                        | ""          | cluster1, cluster2 |
| RUN_ONCE           | Run once and exit (for CronJob use)              | false       | true, false         |

---

### 🔔 Notifications

kube-watchtower integrates with Shoutrrr to send notifications to various services.

Examples

Slack `NOTIFICATION_URL=slack://token-a/token-b/token-c`

Discord `NOTIFICATION_URL=discord://token@channel-id`

Telegram `NOTIFICATION_URL=telegram://token@telegram?chats=@channel-1`

Email (SMTP) `NOTIFICATION_URL=smtp://username:password@smtp.example.com:587/?from=sender@example.com&to=recipient@example.com`

WeChat Work `NOTIFICATION_URL=wechatwork://corpid@token`

For more services, refer to the official Shoutrrr documentation.

---

### 🔍 Monitoring Rules

kube-watchtower monitors containers in Deployments, DaemonSets, and StatefulSets that meet all the following criteria:

	1.	✅ The container's imagePullPolicy is set to Always
	2.	✅ The image tag is latest
	3.	✅ The container is not listed in DISABLE_CONTAINERS

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
- [ ] Notifier formatter(Start log, Update log)
- [ ] CronJob support
- [ ] Rollout timeout support
- [ ] Private registry support
- [ ] Check only mode support
- [ ] Namespace denylist support

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