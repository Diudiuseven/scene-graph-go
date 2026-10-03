// Package mathx provides the small 3D math primitives used by the scene
// package: 3-component vectors, unit quaternions, column-major 4x4 matrices
// and local transforms.
//
// Matrices are column-major (m[col*4+row]) and transforms are evaluated as
// T*R*S (scale first, then rotate, then translate). Quaternions are stored
// as (X, Y, Z, W) with W the scalar part.
package mathx
