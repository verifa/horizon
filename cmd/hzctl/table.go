package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/verifa/horizon/pkg/hz"
	"sigs.k8s.io/yaml"
)

const (
	purple    = lipgloss.Color("99")
	gray      = lipgloss.Color("245")
	lightGray = lipgloss.Color("241")
)

func printObject(object hz.GenericObject) error {
	jb, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("marshalling object: %w", err)
	}
	yb, err := yaml.JSONToYAML(jb)
	if err != nil {
		return fmt.Errorf("converting to yaml: %w", err)
	}

	fmt.Println(string(yb))
	return nil
}

func printObjects(objects []hz.GenericObject) {
	re := lipgloss.NewRenderer(os.Stdout)
	var (
		// HeaderStyle is the lipgloss style used for the table headers.
		HeaderStyle = re.NewStyle().
				Foreground(purple).
				Bold(true).
				Align(lipgloss.Center)
		// CellStyle is the base lipgloss style used for the table rows.
		CellStyle = re.NewStyle().Padding(0, 1).Width(14)
		// OddRowStyle is the lipgloss style used for odd-numbered table rows.
		OddRowStyle = CellStyle.Copy().Foreground(gray)
		// EvenRowStyle is the lipgloss style used for even-numbered table rows.
		EvenRowStyle = CellStyle.Copy().Foreground(lightGray)
		// BorderStyle is the lipgloss style used for the table border.
		BorderStyle = lipgloss.NewStyle().Foreground(purple)
	)

	rows := make([][]string, len(objects))
	for i, obj := range objects {
		rows[i] = []string{
			obj.Kind,
			obj.Account,
			obj.Name,
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(BorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == 0:
				return HeaderStyle
			case row%2 == 0:
				return EvenRowStyle
			default:
				return OddRowStyle
			}
		}).
		Headers(
			"Kind",
			"Account",
			"Name",
		).
		Rows(rows...)

	fmt.Println(t)
}
