package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/stefanoamorelli/estonia-ai-kit/cli/emta/api"
)

func newTSDCommand() *cobra.Command {
	tsdCmd := &cobra.Command{
		Use:   "tsd",
		Short: "Income and social tax return (TSD) operations",
	}

	var listYear string
	tsdListCmd := &cobra.Command{
		Use:   "list",
		Short: "List TSD declarations",
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := loadSession()
			if err != nil {
				return err
			}
			client := api.NewClient(session)

			if listYear == "" {
				listYear = strconv.Itoa(time.Now().Year())
			}

			result, err := client.GetTSDList(listYear)
			if err != nil {
				return fmt.Errorf("fetching TSD list: %w", err)
			}

			if len(result.Declarations) == 0 {
				fmt.Printf("No TSD declarations found for %s\n", listYear)
				return nil
			}

			bold := color.New(color.Bold)
			if result.Person != "" {
				bold.Printf("%s\n", result.Person)
			}
			bold.Printf("TSD Declarations for %s\n\n", listYear)

			t := newTable()
			t.AppendHeader(table.Row{"ID", "Reg.No", "Period", "Submitted", "Status", "Method", "Modified By"})
			for _, d := range result.Declarations {
				period := fmt.Sprintf("%s/%s", d.Year, d.Month)
				t.AppendRow(table.Row{
					d.DeclarationID,
					d.RegNo,
					period,
					d.SubmissionDate,
					d.Status,
					d.Method,
					d.ModifiedBy,
				})
			}

			t.Render()
			return nil
		},
	}
	tsdListCmd.Flags().StringVar(&listYear, "year", "", "Year to list (defaults to current year)")

	tsdShowCmd := &cobra.Command{
		Use:   "show <declaration-id>",
		Short: "Show TSD summary (tax breakdown codes 110-119)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := loadSession()
			if err != nil {
				return err
			}
			client := api.NewClient(session)

			declarationID := args[0]
			summary, err := client.GetTSDSummary(declarationID)
			if err != nil {
				return fmt.Errorf("fetching TSD summary: %w", err)
			}

			bold := color.New(color.Bold)
			cyan := color.New(color.FgCyan, color.Bold)

			cyan.Println("Income and social tax return (TSD)")
			fmt.Println()
			if summary.Person != "" {
				bold.Printf("Person: %s\n", summary.Person)
			}
			fmt.Printf("%s | %s | Status: %s | Currency: %s\n",
				summary.Form, summary.Period, summary.Status, summary.Currency)
			fmt.Println()

			t := newTable()
			t.AppendHeader(table.Row{"", "Code", "Amount (EUR)"})

			for _, line := range summary.Lines {
				t.AppendRow(table.Row{line.Label, line.Code, line.Amount})
			}

			t.Render()
			return nil
		},
	}

	tsdCmd.AddCommand(tsdListCmd, tsdShowCmd)
	return tsdCmd
}
