// Package mathx provides the small 3D math primitives (vectors, quaternions,
// column-major 4x4 matrices and local transforms) used by the scene package.
package mathx

import "math"

// Vec3 is a 3-component vector.
type Vec3 struct {
	X, Y, Z float64
}

// Add returns v+o component-wise.
func (v Vec3) Add(o Vec3) Vec3 {
	return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z}
}

// Sub returns v-o component-wise.
func (v Vec3) Sub(o Vec3) Vec3 {
	return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z}
}

// Scale returns v multiplied by the scalar s.
func (v Vec3) Scale(s float64) Vec3 {
	return Vec3{v.X * s, v.Y * s, v.Z * s}
}

// Dot returns the dot product of v and o.
func (v Vec3) Dot(o Vec3) float64 {
	return v.X*o.X + v.Y*o.Y + v.Z*o.Z
}

// Cross returns the cross product v x o.
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		v.Y*o.Z - v.Z*o.Y,
		v.Z*o.X - v.X*o.Z,
		v.X*o.Y - v.Y*o.X,
	}
}

// Length returns the Euclidean length of v.
func (v Vec3) Length() float64 {
	return math.Sqrt(v.Dot(v))
}

// Normalize returns a unit vector with the same direction. The zero vector is
// returned unchanged.
func (v Vec3) Normalize() Vec3 {
	l := v.Length()
	if l == 0 {
		return v
	}
	return v.Scale(1 / l)
}

// Approx reports whether v and o are component-wise equal within eps.
func (v Vec3) Approx(o Vec3, eps float64) bool {
	return math.Abs(v.X-o.X) <= eps &&
		math.Abs(v.Y-o.Y) <= eps &&
		math.Abs(v.Z-o.Z) <= eps
}
