package codegen

// IdentSanitize converts an IDL name into a valid, exported Go identifier.
// It handles Go reserved words, predeclared identifiers, leading digits,
// hyphens, underscores, and lowercase-leading names.
func IdentSanitize(name string) string {
	panic("IdentSanitize: not yet implemented")
}
