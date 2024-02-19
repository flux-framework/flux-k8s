# Development Notes

## Thinking

> Updated February 15, 2024

What I think might be happening (and not always, sometimes)

- New pod group, no node list
- Fluence assigns nodes
- Nodes get assigned to pods 1:1
- POD group is deleted
- Some pod is sent back to queue (kubelet rejects, etc)
- POD group does not exist and is recreated, no node list
- Fluence asks again, but still has the first job. Not enough resources, asks forever.

The above would not happen with the persistent pod group (if it wasn't cleaned up until the deletion of the job) and wouldn't happen if there are just enough resources to account for the overlap.

- Does Fluence allocate resources for itself?
- It would be nice to be able to inspect the state of Fluence.
- At some point we want to be using the TBA fluxion-go instead of the one off branch we currently have (but we don't need to be blocked for that)
- We should (I think) restore pod group (it's in the controller here) and have our own container built. That way we have total control over the custom resource, and we don't risk it going away.
  - As a part of that, we can add add a mutating webhook that emulates what we are doing in fluence now to find the label, but instead we will create the CRD to hold state instead of trying to hold in the operator.
- It could then also be investigated that we can more flexibly change the size of the group, within some min/max size (also determined by labels?) to help with scheduling.
- Note that kueue has added a Pod Group object, so probably addresses the static case here.
