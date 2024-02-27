package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/verifa/horizon/pkg/hz"
)

const (
	purple    = lipgloss.Color("99")
	gray      = lipgloss.Color("245")
	lightGray = lipgloss.Color("241")
)

func printObject(object hz.GenericObject) error {
	buf := bytes.Buffer{}
	buf.WriteString(
		fmt.Sprintf("apiVersion: %s\n", object.APIVersion),
	)
	buf.WriteString(fmt.Sprintf("kind: %s\n", object.Kind))
	buf.WriteString("meta:\n")
	buf.WriteString(fmt.Sprintf("\taccount: %s\n", object.Account))
	buf.WriteString(fmt.Sprintf("\tname: %s\n", object.Name))
	buf.WriteString(
		fmt.Sprintf("\tmanagedFields: %s\n", object.ManagedFields),
	)
	buf.WriteString(fmt.Sprintf("\tlabels: %s\n", object.Labels))
	buf.WriteString("spec:\n")
	if err := json.Indent(&buf, object.Spec, "", "  "); err != nil {
		return fmt.Errorf("formatting spec: %w", err)
	}
	buf.WriteString("\n\n")
	buf.WriteString("status:\n")
	if object.Status != nil {
		if err := json.Indent(&buf, object.Status, "", "  "); err != nil {
			return fmt.Errorf("formatting status: %w", err)
		}
	}
	fmt.Println(buf.String())
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
