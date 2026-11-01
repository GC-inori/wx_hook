// Package i18n reads the two deployment locale files at startup.
package i18n

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Catalog struct {
	locale   string
	messages map[string]map[string]string
}

func Load(root, locale string) (*Catalog, error) {
	if locale != "zh-CN" && locale != "en-US" {
		return nil, fmt.Errorf("unsupported locale %q", locale)
	}
	c := &Catalog{locale: locale, messages: make(map[string]map[string]string)}
	for _, language := range []string{"zh-CN", "en-US"} {
		path := filepath.Join(root, "locales", language+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load locale %s: %w", language, err)
		}
		var tree map[string]any
		if err := yaml.Unmarshal(data, &tree); err != nil {
			return nil, fmt.Errorf("parse locale %s: %w", language, err)
		}
		flat := make(map[string]string)
		flatten("", tree, flat)
		for _, key := range []string{"errors.success", "errors.invalid_param", "errors.unknown_action", "errors.ipc_error", "errors.timeout", "errors.unsupported_action", "errors.unauthorized", "errors.rate_limit", "errors.method_not_allowed", "errors.request_too_large", "errors.account_not_logged_in", "errors.contact_not_found", "app.ready", "app.shutdown", "launcher.lock_failed", "launcher.start_xvfb", "launcher.start_wechat", "launcher.reuse", "launcher.wait_login", "launcher.logged_in", "launcher.qr_preview", "launcher.qr_ready", "launcher.qr_missing_tool", "launcher.login_timeout", "launcher.linux_only"} {
			if flat[key] == "" {
				return nil, fmt.Errorf("locale %s missing %s", language, key)
			}
		}
		c.messages[language] = flat
	}
	return c, nil
}

func flatten(prefix string, tree map[string]any, output map[string]string) {
	for key, value := range tree {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		switch v := value.(type) {
		case string:
			output[full] = v
		case map[string]any:
			flatten(full, v, output)
		}
	}
}

func (c *Catalog) Text(key string, args ...any) string {
	value := c.messages[c.locale][key]
	if value == "" {
		value = c.messages["en-US"][key]
	}
	if value == "" {
		value = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(value, args...)
	}
	return value
}
