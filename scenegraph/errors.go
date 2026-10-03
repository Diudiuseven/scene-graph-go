package scenegraph

import "errors"

// 可区分的错误原因。
var (
	ErrNodeNotFound   = errors.New("scenegraph: node not found")
	ErrCycleDetected  = errors.New("scenegraph: operation would create a cycle")
	ErrParentDeleted  = errors.New("scenegraph: parent node has been deleted")
	ErrInvalidVersion = errors.New("scenegraph: unsupported serialization version")
)
