package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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

	tsdCmd.AddCommand(newTSDXMLCommand())

	var submitDeclarationID string
	var submitConfirm bool

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

	submitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a TSD draft by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if submitDeclarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			if !submitConfirm {
				return fmt.Errorf("--confirm is required for submit")
			}

			client, err := loadEMTAClient()
			if err != nil {
				return err
			}

			result, err := client.SubmitTSD(submitDeclarationID)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	submitCmd.Flags().StringVar(&submitDeclarationID, "declaration-id", "", "Stable declaration id from tsd list")
	submitCmd.Flags().BoolVar(&submitConfirm, "confirm", false, "Actually submit the declaration")

	tsdCmd.AddCommand(tsdListCmd, tsdShowCmd, submitCmd)
	return tsdCmd
}

func newTSDXMLCommand() *cobra.Command {
	tsdXMLCmd := &cobra.Command{
		Use:   "xml",
		Short: "TSD XML import/export operations",
	}

	var declarationID string
	var outputPath string
	var inputPath string
	var year int
	var month int
	var withSums bool

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export TSD declaration XML",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || outputPath == "" {
				return fmt.Errorf("--declaration-id and --output are required")
			}

			client, err := loadEMTAClient()
			if err != nil {
				return err
			}

			result, err := client.ExportTSDXML(declarationID)
			if err != nil {
				return err
			}

			if err := os.WriteFile(outputPath, result.Bytes, 0o600); err != nil {
				return err
			}

			return printJSON(map[string]string{
				"declaration_id": result.DeclarationID,
				"output":         outputPath,
				"file_name":      filepath.Base(result.FileName),
			})
		},
	}
	exportCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from tsd list")
	exportCmd.Flags().StringVar(&outputPath, "output", "", "Path to write XML file")

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import TSD XML into a new draft",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputPath == "" {
				return fmt.Errorf("--input is required")
			}
			if year == 0 || month == 0 {
				return fmt.Errorf("--year and --month are required")
			}

			xmlBytes, err := os.ReadFile(inputPath)
			if err != nil {
				return err
			}

			client, err := loadEMTAClient()
			if err != nil {
				return err
			}

			result, err := client.CreateTSDDraftFromXML(year, month, filepath.Base(inputPath), xmlBytes, withSums)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	importCmd.Flags().StringVar(&inputPath, "input", "", "Path to XML input file")
	importCmd.Flags().IntVar(&year, "year", 0, "Tax year")
	importCmd.Flags().IntVar(&month, "month", 0, "Tax month (1-12)")
	importCmd.Flags().BoolVar(&withSums, "with-sums", false, "Import appendix 1, appendix 2 and INF1 together with tax sums")

	tsdXMLCmd.AddCommand(exportCmd, importCmd)
	return tsdXMLCmd
}
