package mathx

// Transform is a local transform composed of position, rotation and scale.
// Its matrix is evaluated as T * R * S (scale first, then rotate, then
// translate), matching the usual glTF / Unity convention.
type Transform struct {
	Position Vec3
	Rotation Quat
	Scale    Vec3
}

// DefaultTransform returns an identity transform (no translation, rotation or
// scaling).
func DefaultTransform() Transform {
	return Transform{
		Rotation: IdentityQuat(),
		Scale:    Vec3{X: 1, Y: 1, Z: 1},
	}
}

// Mat4 returns the local transform matrix T*R*S.
func (t Transform) Mat4() Mat4 {
	s := ScaleMat4(t.Scale)
	r := t.Rotation.ToMat4()
	tr := TranslationMat4(t.Position)
	return tr.Mul(r).Mul(s)
}
