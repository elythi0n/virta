# Workspaces and Profiles

A workspace (also called a profile) is a saved set of channels, filter rules, and settings. Workspaces let you switch between different streaming setups instantly — for example, a gaming workspace with one set of channels and filters vs. a talk show workspace with different ones.

---

## Default workspace

Virta starts with one workspace called **default**. Every channel you add and every rule you create goes into the active workspace.

---

## Creating a workspace

1. Click the current workspace name in the top-left of the title bar (next to "Virta").
2. In the dropdown, type a name in the **New workspace name** field.
3. Press Enter or click the **+** button.

The new workspace starts empty. Channels and rules are per-workspace.

---

## Switching workspaces

Click the workspace name in the title bar and select a different workspace. Virta:

- Disconnects channels that are only in the previous workspace
- Joins channels that are in the new workspace
- Applies the new workspace's filter rules

Channels shared between the two workspaces stay connected with no gap.

---

## Deleting a workspace

Open the workspace dropdown. Delete buttons (×) appear on hover next to non-default, non-active workspaces.

The **default** workspace and the **currently active** workspace cannot be deleted.

---

## What a workspace contains

| Setting | Per-workspace? |
|---|---|
| Channel list | Yes |
| Filter rules | Yes |
| Platform connection mode (IRC / EventSub) | Yes |
| Message logging policy | Yes |
| Panel layout | Not yet (shared across workspaces in alpha) |

---

## Workspaces in hosted mode

In [hosted multi-user mode](Hosted-Multi-User-Mode), each user has their own set of workspaces entirely separate from other users. There is no workspace sharing between accounts.
