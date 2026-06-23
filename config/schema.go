package config

type AppConfig struct {
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
	Model    string `json:"model"`
	Theme    string `json:"theme"`
}

func DefaultConfig() *AppConfig {
	return &AppConfig{
		APIURL:   "https://api.openai.com/v1",
		APIToken: "",
		Model:    "gpt-4o",
		Theme:    "dark",
	}
}
