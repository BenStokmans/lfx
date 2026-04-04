package lower

// MangleName creates a unique flattened name for an imported function.
// e.g. "helpers", "half" -> "helpers__half"
func MangleName(moduleAlias, funcName string) string {
	return moduleAlias + "__" + funcName
}
