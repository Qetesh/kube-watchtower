[![kube-watchtower docker image](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml/badge.svg)](https://github.com/Qetesh/kube-watchtower/actions/workflows/Packages.yml)


# kube-watchtower

kube-watchtower is a Kubernetes-native image update monitor inspired by Watchtower.
It automatically tracks container image updates within your Kubernetes cluster and safely performs rolling updates when new images are detected.

⚠️ kube-watchtower is currently in beta and not recommended for production use.

### ✨ Features
- Monitors image updates in Deployments, DaemonSets, and StatefulSets
- Detects changes across all tags and private registries
- Performs safe, automated rolling updates on new image digests
- Supports notifications through Shoutrrr
- Optional CronJob scheduling and namespace denylist

---

## 🚀 Getting Started

### Deployment [kube-watchtower.yaml](./CronJob/kube-watchtower.yaml)
- Configure settings via the kube-watchtower-config ConfigMap.
- Adjust the update schedule in the CronJob's schedule field.
- Apply the provided `kube-watchtower.yaml` to your Kubernetes cluster.
- After deployment, a CronJob named kube-watchtower will be created automatically.

#### To run the CronJob immediately, manually trigger the CronJob
```bash
kubectl create job --from=cronjob/kube-watchtower kube-watchtower-manual-$(date +%s) -n kube-watchtower
```

#### For Cron syntax details, refer to:
- [Kubernetes CronJob schedule](https://kubernetes.io/zh-cn/docs/concepts/workloads/controllers/cron-jobs/)
- [crontab.guru](https://crontab.guru)
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
- ✅ The container has available replicas
- ✅ The namespace is not listed in DISABLE_NAMESPACES
- ✅ ImagePullSecret is set up for the private Docker registry

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