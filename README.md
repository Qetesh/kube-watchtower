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

<img width="2356" height="1294" alt="ScreenShot" src="https://github.com/user-attachments/assets/b3b588f7-6038-4f0d-ba54-7c5b330c4877" />

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
| ENABLE_NAMESPACES  | Comma-separated allowlist of namespaces (if set, only these namespaces are monitored) | "" | production,staging |
| DISABLE_NAMESPACES | Comma-separated denylist of namespaces (ignored if ENABLE_NAMESPACES is set) | "" | kube-system,default |
| NOTIFICATION_URL   | Notification URL (Shoutrrr format)               | ""          | See below           |
| NOTIFICATION_CLUSTER | Notification cluster name                      | kubernetes  | cluster1, cluster2  |
| LOG_LEVEL          | Log level (debug, info, warn, error)             | info        | debug, info         |
| DRY_RUN            | Enable dry-run mode (detect but not update)      | false       | true, false         |

---

### 🔔 Notifications

kube-watchtower integrates with [Shoutrrr](https://containrrr.dev/shoutrrr/) to send notifications to various services.

---

### 🔍 Monitoring Rules

kube-watchtower monitors containers in Deployments, DaemonSets, and StatefulSets that meet all the following criteria:

- ✅ The container's imagePullPolicy is set to Always
- ✅ The container has available replicas
- ✅ The namespace passes the allowlist/denylist filter (see below)
- ✅ ImagePullSecret is set up for the private Docker registry

**Namespace Filtering:**
- If `ENABLE_NAMESPACES` is set, only namespaces in this list will be monitored (allowlist mode)
- If `ENABLE_NAMESPACES` is empty, all namespaces except those in `DISABLE_NAMESPACES` will be monitored (denylist mode)

---

### 📝 Todo

- [x] Deployments, DaemonSet, StatefulSets
- [x] Notifier formatter(Start log, Update log)
- [x] CronJob support
- [x] Private registry support via ImagePullSecrets
- [x] Rolling update timeout support
- [x] Namespace allowlist/denylist support
- [x] Dry-run mode support
- [ ] [Garbage Collection](https://kubernetes.io/docs/concepts/architecture/garbage-collection/) Suggestions are welcome

---

### ❓ FAQ

Q: My container isn't being monitored. Why?

Ensure that imagePullPolicy is set to Always, and the namespace is not listed in DISABLE_NAMESPACES.

Q: Can I monitor private registries?

Yes. Make sure your cluster is configured with valid ImagePullSecrets.
kube-watchtower automatically uses the Pod's service account credentials.

Q: What happens if the update doesn’t complete?

GitOps tools like ArgoCD may automatically self-heal resources, reverting changes before the rollout finishes.
This can prevent all Pods from updating successfully.
You may need to temporarily disable self-heal during the update.

Q: How do I control which namespaces to monitor?

There are two modes:

**allowlist Mode (recommended for production):**
Set `ENABLE_NAMESPACES` to only monitor specific namespaces.
Example: `ENABLE_NAMESPACES=production,staging`

**denylist Mode:**
Leave `ENABLE_NAMESPACES` empty and use `DISABLE_NAMESPACES` to exclude specific namespaces.
Example: `DISABLE_NAMESPACES=kube-system,kube-public,default`

Note: If `ENABLE_NAMESPACES` is set, `DISABLE_NAMESPACES` is ignored.

Q: Can I test without actually updating containers?

Yes. Enable DRY_RUN mode by setting `DRY_RUN=true`. In this mode, kube-watchtower will:
- Detect and report available image updates
- Skip the actual rollout restart operations
- Send notifications with [DRY-RUN] label showing detected updates

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