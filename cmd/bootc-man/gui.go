package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Manage the web GUI service",
	Long: `Manage the web GUI service for bootc-man.

Note: The GUI service is planned for a future release.
Currently, this command is a placeholder.`,
}

var guiUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the GUI service",
	RunE:  runGUIUp,
}

var guiDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the GUI service",
	RunE:  runGUIDown,
}

var guiStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GUI service status",
	RunE:  runGUIStatus,
}

func init() {
	guiCmd.AddCommand(guiUpCmd)
	guiCmd.AddCommand(guiDownCmd)
	guiCmd.AddCommand(guiStatusCmd)
}

func runGUIUp(cmd *cobra.Command, args []string) error {
	fmt.Println("⚠️  GUI service is not yet implemented.")
	fmt.Println("   This feature is planned for a future release.")
	fmt.Printf("   Configured port: %d\n", getConfig().GUI.Port)
	return nil
}

func runGUIDown(cmd *cobra.Command, args []string) error {
	fmt.Println("⚠️  GUI service is not yet implemented.")
	return nil
}

func runGUIStatus(cmd *cobra.Command, args []string) error {
	cfg := getConfig()
	fmt.Println("GUI Service Status: not implemented")
	fmt.Printf("Configured Port: %d\n", cfg.GUI.Port)
	return nil
}
