# scene-graph-go

A local-process 3D scene graph: nodes arranged in a tree, each carrying a
local transform whose world transform is the product of the local transforms
along the path to the root. The system is self-contained (no external
services); input is simulated in memory or from local files.

## Features

- **Hierarchy & local transforms** — parent/child tree with per-node
  position, rotation and scale.
- **Frequent structural edits** — create, delete (children are reparented to
  their grandparent), reparent and reorder siblings.
- **Lazy world-transform cache** — a dirty flag propagates along ancestor
  chains; only the dirty portion of a path is recomputed on read, and
  unaffected subtrees are never touched.
- **Concurrent reads and writes** — one `sync.RWMutex` guards all state.
  Clean reads take the read lock; reads that must recompute upgrade to the
  write lock. Every read returns the world transform of one self-consistent
  tree state; no half-updated results and no lock-order deadlocks.
- **Illegal-structure rejection** — cycles, references to deleted parents,
  and root edits are rejected with distinguishable errors (`ErrCycle`,
  `ErrParentNotFound`, `ErrInvalidOperation`). Failed edits leave the tree
  unchanged.
- **Versioned serialization** — current format (v2) and legacy format (v1)
  are both readable; older data is migrated forward and validated, with
  equivalent world transforms restored.
- **Reproducibility** — deterministic node ids and breadth-first output make
  a fixed operation sequence produce identical world transforms across runs.

## Transform semantics

A node's local transform is `{position, rotation, scale}`. Its local matrix
is evaluated as

```
localMatrix = T(position) * R(rotation) * S(scale)
```

i.e. scale is applied first, then rotation, then translation (the glTF /
Unity convention). Matrices are column-major (`m[col*4+row]`), so the world
matrix of a node is

```
worldMatrix = parent.worldMatrix * node.localMatrix
```

and the world matrix of the root equals its local matrix. Rotations are unit
quaternions stored as `(X, Y, Z, W)`.

## Dirty-cache semantics

- Writing a node's local transform marks that node dirty.
- Reparenting a node marks that node dirty.
- Creating a node marks the new node dirty.
- Deleting a node marks its reparented children dirty.
- Reordering siblings does **not** mark anything dirty (order does not affect
  world transforms).
- Descendants are **not** eagerly marked; they are recomputed lazily when
  read. A read walks up from the node to the topmost dirty ancestor and
  recomputes top-down, so only the dirty portion of the path is evaluated.
- `Snapshot` / `SnapshotFull` recompute every node breadth-first under a
  single lock, producing one self-consistent view of the whole tree.

## Concurrency guarantees

- All scene state is guarded by a single `sync.RWMutex`.
- `WorldTransform` fast path: if the node and all its ancestors are clean,
  the cached world matrix is returned under the read lock.
- Slow path: the read lock is released and the write lock acquired; the
  dirty path is then recomputed. No lock is held while waiting on another
  lock, so there is no lock-order inversion and no deadlock.
- Every returned world transform corresponds to the tree state at the moment
  the lock was acquired; a reader never observes a half-updated tree.
- `SnapshotFull` returns local and world transforms plus parent/children in
  one locked pass, so callers can verify parent-chain consistency without
  racing against writers.

## Error semantics

| Error | Meaning |
| --- | --- |
| `ErrNodeNotFound` | The referenced node does not exist. |
| `ErrParentNotFound` | A node is attached to a parent that does not exist (e.g. it was deleted). |
| `ErrCycle` | The operation would create a parent cycle (self-parenting or reparenting under a descendant). |
| `ErrInvalidOperation` | An edit that is never legal (delete/reparent/reorder the root, sibling index out of range). |
| `ErrCorruptData` | Serialized input is not valid scene data. |
| `ErrUnsupportedVersion` | Serialized data uses a format version this build cannot read. |

Failed edits leave the tree in its previous state.

## Serialization

- `Save(w)` writes the current format (v2): string node ids, explicit root
  with empty `parentId`, and explicit ordered `children` lists.
- `SaveV1(w)` writes the legacy format (v1): integer ids with the root as id
  0 and per-node sibling order. It exists for cross-version compatibility
  testing.
- `Load(r)` auto-detects the version, migrates older formats forward,
  validates the structure (rejecting cycles and references to deleted
  parents), and returns a ready-to-use scene.

The on-disk JSON for v2 looks like:

```json
{
  "version": 2,
  "nodes": [
    {
      "id": "root",
      "name": "root",
      "parentId": "",
      "children": ["n1"],
      "local": {
        "position": [0, 0, 0],
        "rotation": [0, 0, 0, 1],
        "scale": [1, 1, 1]
      }
    }
  ]
}
```

## API overview

```go
s := scene.NewScene()
root := s.Root()

a, _ := s.CreateNode(root, "a", mathx.Transform{...})
b, _ := s.CreateNode(a, "b", mathx.Transform{...})

s.SetLocalTransform(a, mathx.Transform{...})
s.SetParent(b, root)
s.SetSiblingIndex(b, 0)
s.DeleteNode(a)

w, _ := s.WorldTransform(b)        // lazy, consistent
snap, _ := s.SnapshotFull()        // self-consistent whole-tree view
_ = s.Validate()                   // integrity check

var buf bytes.Buffer
s.Save(&buf)                       // current format
loaded, _ := scene.Load(&buf)      // auto-detect + migrate
```

## Testing

```sh
go test ./...          # all tests
go test -race ./...    # with the race detector
```

The test suite covers:

- world-transform composition along parent chains, including rotation;
- structural edits (create, delete with grandparent reparenting, reparent,
  sibling reorder) and their immediate effect on world transforms;
- dirty-cache behavior (unaffected nodes are not recomputed);
- illegal-structure rejection (cycles, deleted parents, root edits) with
  distinguishable errors and unchanged tree state;
- concurrent reads and writes under the race detector, verifying that every
  snapshot satisfies `world[child] == world[parent] * local[child]`;
- determinism across runs;
- serialization round-trips and v1→v2 migration with equivalent world
  transforms, plus rejection of corrupt data and unsupported versions.
