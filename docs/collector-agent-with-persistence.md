# Pros and cons of using OTel Collector agent with local persistence

## Pros

- App-to-agent communication is local using <https://kubernetes.io/docs/reference/networking/virtual-ips/#internal-traffic-policy>, which is
  more reliable than communication between pods on different nodes. This allows you to shift persistence to the agent side.
- Enables use of the node file system for persistence. This is operationally straightforward (implementing filesystem buffering on the app
  side would require StatefulSets for all applications). However, there are also disadvantages - see cons section.

## Cons

- Node file system is less reliable than distributed storage. If the node fails, all persisted data may be lost. Additionally, snapshotting
  capabilities with node file systems are uncertain.
- Limited to vertical scaling only. Large persistence requirements necessitate nodes with substantial disk space. Distributed storage allows
  horizontal scaling by adding nodes to the storage cluster.
- When the agent is unavailable (during rolling upgrades), applications will drop data by default. However, the retry mechanism can be
  fine-tuned to mitigate this.

## Deployment Topologies

![Deployment Topologies](assets/otel-deployment-topologies.drawio.svg)
