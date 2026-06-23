// Package model holds the shell-agnostic data types shared across the engine.
package model

// Category is one bucket in the (load-order) taxonomy.
type Category string

const (
	CatEnvironment Category = "environment"
	CatPath        Category = "path"
	CatSecrets     Category = "secrets"
	CatPlugins     Category = "plugins"
	CatOptions     Category = "options"
	CatKeybindings Category = "keybindings"
	CatFunctions   Category = "functions"
	CatAliases     Category = "aliases"
	CatLocal       Category = "local"
	CatMisc        Category = "misc"
)

// Categories returns the categories in taxonomy (load) order.
func Categories() []Category {
	return []Category{
		CatEnvironment, CatPath, CatSecrets, CatPlugins, CatOptions,
		CatKeybindings, CatFunctions, CatAliases, CatLocal, CatMisc,
	}
}

// Description is a human-readable label for a category.
func (c Category) Description() string {
	switch c {
	case CatEnvironment:
		return "Environment variables / exports"
	case CatPath:
		return "PATH / fpath manipulation"
	case CatSecrets:
		return "Secrets & tokens (do not sync)"
	case CatPlugins:
		return "Plugin managers, framework & tool init"
	case CatOptions:
		return "Shell options (setopt/zstyle/autoload)"
	case CatKeybindings:
		return "Key bindings (bindkey)"
	case CatFunctions:
		return "Function definitions"
	case CatAliases:
		return "Aliases"
	case CatLocal:
		return "Machine / OS-specific overrides"
	case CatMisc:
		return "Uncategorized — review by hand"
	default:
		return "Unknown category"
	}
}
