# scene-graph-go

进程内的三维场景图与层级变换系统：组织节点父子层级，维护局部/世界变换，
在频繁结构编辑与并发访问下保证世界变换一致且高效。

## 快速开始

```go
s := scenegraph.NewScene()

root, _ := s.CreateNode("root", 0, scenegraph.IdentityTransform())
child, _ := s.CreateNode("child", root, scenegraph.Transform{
    Position: scenegraph.Vec3{X: 1},
    Rotation: scenegraph.Quat{W: 1},
    Scale:    scenegraph.Vec3{1, 1, 1},
})

world, _ := s.WorldTransform(child)   // 父链组合结果，带缓存
_ = s.SetLocalTransform(child, t)     // 写入局部变换
_ = s.Reparent(child, otherParent)    // 重新挂载
_ = s.ReorderChild(root, child, 0)    // 调整同级顺序
_ = s.DeleteNode(child)               // 删除整棵子树

data, _ := s.Marshal()                // 序列化
s2, _ := scenegraph.Unmarshal(data)   // 反序列化（兼容旧版本）
```

## 变换语义

- 局部变换为 TRS（平移 `Position`、旋转 `Rotation` 四元数、缩放 `Scale`），
  组合顺序为 `M = T * R * S`（先缩放、再旋转、再平移）。
- 节点的世界变换等于从根到该节点的父链上各局部矩阵依次左乘：
  `W(node) = W(parent) * M(local)`，根节点的世界变换即其局部变换。
- 矩阵为 4x4 列主序（`m[col*4+row]`），`Mat4.Mul` 满足 `a.Mul(b)` 表示先应用 `b` 再应用 `a`。
- 四元数在转矩阵前自动归一化，零四元数按单位旋转处理。

## 一致性保证

- **原子性**：所有公开方法共用一把 `sync.RWMutex`。任何一次读取观察到的都是
  某个完整提交后的树状态，不会看到结构编辑或变换写入的中间结果。
- **无死锁**：锁的获取只有一处（方法入口），不存在锁层级与嵌套加锁；
  内部递归（世界变换重算、脏标记）在持锁状态下进行，不再次取锁。
- **缓存一致性**：世界变换按需计算并缓存。结构编辑（增删、重挂载）与
  局部变换写入只将**受影响的子树**标记为脏；未受影响节点的缓存保持有效，
  不会被重算。不变量：节点为脏 ⇒ 其后代必然已为脏，因此脏标记可提前剪枝。
- **并发安全**：多个读者可并行；写操作互斥。`WorldTransform` 为填充缓存
  会获取写锁，但对同一状态的重复读取是幂等的，结果不变。

## 非法结构处理

- `Reparent` 若会产生环路（目标父节点是被移动节点的后代或自身），
  返回 `ErrCycleDetected`。
- 以已删除节点为父（`CreateNode` / `Reparent`）返回 `ErrParentDeleted`。
- 引用不存在的节点返回 `ErrNodeNotFound`。
- 所有校验在修改树之前完成，被拒绝的操作不会留下中间状态。

## 序列化与版本兼容

格式：4 字节魔数 `SGPH` + 4 字节小端版本号 + JSON 负载。

- **v2**（当前）：保存有序的子节点列表与 `next_id`，完整还原同级顺序与 ID 分配。
- **v1**：不保存同级顺序；读取时自动升级，同级顺序按节点 ID 升序重建，
  世界变换与层级关系完全等价。
- 未知版本或损坏数据返回 `ErrInvalidVersion`。
- 序列化输出按节点 ID 排序，同一状态多次序列化字节完全一致（可复现）。

## 可复现性

固定输入与固定操作序列下，多次运行的最终世界变换逐节点一致，
序列化字节流也一致（节点 ID 由单调计数器分配，遍历均排序）。

## 测试

```sh
go test -race ./...
```

覆盖：结构编辑（增删/重挂载/排序）、缓存失效范围、并发读写（race detector）、
环路/已删除父节点拒绝、v1/v2 序列化兼容与升级、确定性复现。
