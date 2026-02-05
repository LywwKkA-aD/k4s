# Views

## Namespaces View (`1`)

Browse and select Kubernetes namespaces.

**Columns:**
- Namespace name
- Status
- Age

## Pods View (`2`)

List all pods in the selected namespace.

**Columns:**
- Pod name
- Ready containers (X/Y)
- Status (color-coded)
- Restart count
- Age
- CPU/Memory (toggle with `m`, requires metrics-server)

**Features:**
- Auto-refreshes every 5 seconds
- Color-coded status (Running=green, Pending=yellow, Failed=red)

**Actions:** `l` logs, `d` delete, `R` restart, `m` metrics

## Pod Details (Enter on pod)

Detailed information about a pod.

**Sections:**
- Metadata (labels, annotations)
- Container information
- Resource requests/limits
- Recent events

## Deployments View (`3`)

List all deployments in the selected namespace.

**Columns:**
- Deployment name
- Ready/Desired replicas
- Up-to-date count
- Available count
- Age

**Actions:** `s` scale, `d` delete, `R` restart

## Services View (`4`)

List all services in the selected namespace.

**Columns:**
- Service name
- Type (ClusterIP, NodePort, LoadBalancer)
- Cluster IP
- External IP
- Ports
- Age

## Events View (`5`)

Cluster-wide events in log-style format.

**Features:**
- Auto-follow mode (`f`)
- Filter warnings only (`w`)
- Filter by resource kind (`k`)
- Color-coded by event type (Normal=blue, Warning=yellow)

## Log Viewer (`l`)

View pod logs with streaming support.

**Features:**
- Follow mode for real-time logs
- Multi-container support (press `c` to switch)
- Timestamp toggle (`t`)
- Search with highlighting (`/`, `n`, `N`)
- ANSI color preservation

## SSH Hosts View (`9`)

Connect to K3s nodes via SSH.

**Features:**
- View containers on the node via crictl
- Inspect container logs
- See node system information

See [SSH Integration](ssh.md) for setup details.
