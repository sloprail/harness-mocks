package runner

// projectDirOf is the project root the run's hooks are told: the one given, else
// the working directory.
func projectDirOf(cfg Config) string {
	if cfg.ProjectDir != "" {
		return cfg.ProjectDir
	}
	return cfg.Cwd
}
