package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the full castle-cli configuration.
type Config struct {
	Environment string           `mapstructure:"environment"`
	Port        int              `mapstructure:"port"`
	DefectDojo  DefectDojoConfig `mapstructure:"defectdojo"`
	Kubernetes  KubernetesConfig `mapstructure:"kubernetes"`
	ArgoCD      ArgoCDConfig     `mapstructure:"argocd"`
}

// DefectDojoConfig holds DefectDojo API connection settings.
type DefectDojoConfig struct {
	URL    string `mapstructure:"url"`
	APIKey string `mapstructure:"api_key"`
}

// KubernetesConfig holds Kubernetes cluster settings.
type KubernetesConfig struct {
	Namespace  string `mapstructure:"namespace"`
	Kubeconfig string `mapstructure:"kubeconfig"`
}

// ArgoCDConfig holds ArgoCD API connection settings used by castle deploy.
type ArgoCDConfig struct {
	// URL is the base URL of the ArgoCD API server, e.g. https://argocd.example.com.
	URL string `mapstructure:"url"`
	// Token is an ArgoCD API token with sufficient permissions to trigger syncs.
	// Prefer setting CASTLE_ARGOCD_TOKEN in the environment over storing it here.
	Token string `mapstructure:"token"`
}

// Load reads configuration from cfgFile (if non-empty) or searches for
// castle.yaml in $HOME and the current working directory.
// Defaults are applied for every key that is not explicitly set; environment
// variables prefixed with CASTLE_ always override file values.
func Load(cfgFile string) (Config, error) {
	v := viper.New()
	setDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("config: resolving home directory: %w", err)
		}
		v.AddConfigPath(home)
		v.AddConfigPath(".")
		v.SetConfigName("castle")
		v.SetConfigType("yaml")
	}

	// CASTLE_PORT, CASTLE_DEFECTDOJO_API_KEY, CASTLE_ARGOCD_TOKEN, …
	v.SetEnvPrefix("CASTLE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			slog.Info("config: no config file found, using defaults and environment variables")
		} else {
			return Config{}, fmt.Errorf("config: reading config file: %w", err)
		}
	} else {
		slog.Info("config: loaded", "file", v.ConfigFileUsed())
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: unmarshalling configuration: %w", err)
	}

	return cfg, nil
}

// setDefaults registers sane defaults for every configuration key.
func setDefaults(v *viper.Viper) {
	v.SetDefault("environment", "development")
	v.SetDefault("port", 8080)
	v.SetDefault("defectdojo.url", "http://localhost:8000")
	v.SetDefault("defectdojo.api_key", "")
	v.SetDefault("kubernetes.namespace", "default")
	v.SetDefault("kubernetes.kubeconfig", "")
	v.SetDefault("argocd.url", "")
	v.SetDefault("argocd.token", "")
}
