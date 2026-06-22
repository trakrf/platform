package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/urfave/cli/v3"

	"github.com/trakrf/platform/cli/internal/apiclient"
	"github.com/trakrf/platform/cli/internal/config"
)

func authCommand() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Manage API credentials and profiles",
		Commands: []*cli.Command{
			{
				Name:  "login",
				Usage: "Store API credentials and verify them against the API",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "client-id", Usage: "API key client_id"},
					&cli.StringFlag{Name: "client-secret", Usage: "API key client_secret"},
					&cli.BoolFlag{Name: "no-input", Usage: "fail instead of prompting for missing values"},
				},
				Action: runAuthLogin,
			},
			{
				Name:   "logout",
				Usage:  "Remove stored credentials for the active (or --profile) profile",
				Action: runAuthLogout,
			},
			{
				Name:   "status",
				Usage:  "Show the active profile and whether it is authenticated",
				Action: runAuthStatus,
			},
		},
	}
}

func runAuthLogin(ctx context.Context, cmd *cli.Command) error {
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	profileName := cmd.String("profile")
	if profileName == "" {
		profileName = "default"
	}
	env := cmd.String("env")
	clientID := cmd.String("client-id")
	clientSecret := cmd.String("client-secret")

	// Seed prompts with any existing values for this profile.
	if existing, ok := cfg.Profiles[profileName]; ok {
		if env == "" {
			env = existing.Env
		}
		if clientID == "" {
			clientID = existing.ClientID
		}
	}
	if env == "" {
		env = "prod"
	}

	needPrompt := clientID == "" || clientSecret == ""
	if needPrompt {
		if cmd.Bool("no-input") {
			return fmt.Errorf("missing --client-id/--client-secret and --no-input is set")
		}
		if err := promptCredentials(&env, &clientID, &clientSecret); err != nil {
			return err
		}
	}
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("client-id and client-secret are required")
	}

	baseURL, err := resolveBaseURL(env)
	if err != nil {
		return err
	}

	// Verify the credentials by minting a token before persisting anything.
	minter := &apiclient.Minter{BaseURL: baseURL}
	tok, err := minter.Mint(ctx, clientID, clientSecret)
	if err != nil {
		return fmt.Errorf("verifying credentials: %w", err)
	}

	cfg.Profiles[profileName] = &config.Profile{
		Env:          env,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Token:        &tok,
	}
	cfg.CurrentProfile = profileName
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Logged in to %s as profile %q (%s).\n", baseURL, profileName, env)
	return nil
}

func promptCredentials(env, clientID, clientSecret *string) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Environment").
				Options(
					huh.NewOption("Production (app.trakrf.id)", "prod"),
					huh.NewOption("Preview (app.preview.trakrf.id)", "preview"),
				).
				Value(env),
			huh.NewInput().
				Title("Client ID").
				Value(clientID),
			huh.NewInput().
				Title("Client secret").
				EchoMode(huh.EchoModePassword).
				Value(clientSecret),
		),
	)
	return form.Run()
}

func runAuthLogout(ctx context.Context, cmd *cli.Command) error {
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	name := cmd.String("profile")
	if name == "" {
		name = cfg.CurrentProfile
	}
	if name == "" {
		return fmt.Errorf("no active profile to log out of")
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	delete(cfg.Profiles, name)
	if cfg.CurrentProfile == name {
		cfg.CurrentProfile = ""
	}
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Removed credentials for profile %q.\n", name)
	if os.Getenv("TRAKRF_API_KEY") != "" {
		fmt.Fprintln(os.Stdout, "Note: TRAKRF_API_KEY is still set and will be used for API calls.")
	}
	return nil
}

func runAuthStatus(ctx context.Context, cmd *cli.Command) error {
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	name, prof, err := cfg.Resolve(config.ResolveInput{
		Profile:   cmd.String("profile"),
		Env:       cmd.String("env"),
		OrgEnv:    os.Getenv("TRAKRF_ORG"),
		APIKeyEnv: os.Getenv("TRAKRF_API_KEY"),
	})
	if err != nil {
		fmt.Fprintln(os.Stdout, "Not logged in. Run `trakrf auth login` or set TRAKRF_API_KEY.")
		return nil
	}

	baseURL, err := resolveBaseURL(prof.Env)
	if err != nil {
		return err
	}

	w := os.Stdout
	fmt.Fprintf(w, "Profile:     %s\n", name)
	fmt.Fprintf(w, "Environment: %s (%s)\n", prof.Env, baseURL)
	if prof.ClientID != "" {
		fmt.Fprintf(w, "Client ID:   %s\n", prof.ClientID)
	}
	if os.Getenv("TRAKRF_API_KEY") != "" {
		fmt.Fprintln(w, "Credentials: from TRAKRF_API_KEY (environment override)")
	}
	if prof.ClientID == "" || prof.ClientSecret == "" {
		fmt.Fprintln(w, "Status:      no credentials — run `trakrf auth login`")
		return nil
	}

	// Best-effort live check: resolve a token and read the org.
	rc, err := newRunCtx(cmd)
	if err != nil {
		fmt.Fprintf(w, "Status:      credentials present, but client setup failed: %v\n", err)
		return nil
	}
	resp, err := rc.client.GetCurrentOrgWithResponse(ctx)
	switch {
	case err != nil:
		fmt.Fprintf(w, "Status:      could not reach API: %v\n", err)
	case resp.JSON200 != nil:
		fmt.Fprintf(w, "Status:      authenticated as org %q (id %s)\n", resp.JSON200.Data.Name, id64(resp.JSON200.Data.Id))
		fmt.Fprintf(w, "Scopes:      %v\n", resp.JSON200.Data.Scopes)
	default:
		fmt.Fprintf(w, "Status:      not authenticated: %v\n", apiclient.APIError(resp.StatusCode(), resp.Body))
	}
	return nil
}
