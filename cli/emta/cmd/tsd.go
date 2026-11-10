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

	tsdCmd.AddCommand(newTSDXMLCommand(), newTSDCSVCommand(), newTSDPaymentsCommand())

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
			if err := requireSubmitPermission(submitConfirm); err != nil {
				return err
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

// newTSDCSVCommand builds the transitional payment CSV without contacting EMTA.
func newTSDCSVCommand() *cobra.Command {
	command := &cobra.Command{Use: "csv", Short: "Transitional resident payroll CSV"}
	var input, output string
	build := &cobra.Command{Use: "build", Short: "Build appendix 1 payroll CSV from JSON", RunE: func(cmd *cobra.Command, args []string) error {
		if input == "" || output == "" {
			return fmt.Errorf("--input and --output are required")
		}
		data, err := os.ReadFile(input)
		if err != nil {
			return err
		}
		in, err := api.ParseTSDPayrollCSVInput(data)
		if err != nil {
			return err
		}
		csv, err := api.BuildTSDPayrollCSV(in)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		if _, err = file.Write(csv); err != nil {
			file.Close()
			os.Remove(output)
			return err
		}
		if err = file.Close(); err != nil {
			os.Remove(output)
			return err
		}
		return printJSON(map[string]any{"tax_payer_reg_code": in.TaxPayerRegCode, "year": in.Year, "month": in.Month, "payments": len(in.Payments), "output": output})
	}}
	build.Flags().StringVar(&input, "input", "", "Path to payroll JSON input")
	build.Flags().StringVar(&output, "output", "", "Path to new CSV file")
	command.AddCommand(build)
	return command
}

// newTSDPaymentsCommand uploads and reads appendix 1 payments in the payments view (periods from 2026-10).
func newTSDPaymentsCommand() *cobra.Command {
	command := &cobra.Command{Use: "payments", Short: "TSD payments view (appendix 1 from 2026-10)"}
	var declarationID, input string
	upload := &cobra.Command{Use: "upload", Short: "Upload payroll JSON as transitional CSV into a TSD draft, then read payments back", RunE: func(cmd *cobra.Command, args []string) error {
		if declarationID == "" || input == "" {
			return fmt.Errorf("--declaration-id and --input are required")
		}
		data, err := os.ReadFile(input)
		if err != nil {
			return err
		}
		in, err := api.ParseTSDPayrollCSVInput(data)
		if err != nil {
			return err
		}
		csv, err := api.BuildTSDPayrollCSV(in)
		if err != nil {
			return err
		}
		session, err := loadSession()
		if err != nil {
			return err
		}
		client := api.NewClient(session)
		if err := client.CheckTSDDraftOwner(declarationID, in.TaxPayerRegCode, in.Year, in.Month); err != nil {
			return err
		}
		uploaded, err := client.UploadTSDPaymentsCSV(declarationID, fmt.Sprintf("tsd-%d-%02d.csv", in.Year, in.Month), csv)
		if err != nil {
			return err
		}
		readback, err := client.ReadTSDPayments(declarationID)
		if err != nil {
			return fmt.Errorf("uploaded, but readback failed: %w", err)
		}
		return printJSON(map[string]any{"declaration_id": declarationID, "upload": uploaded, "readback": readback})
	}}
	upload.Flags().StringVar(&declarationID, "declaration-id", "", "Declaration id from tsd list")
	upload.Flags().StringVar(&input, "input", "", "Path to payroll JSON input (same format as csv build)")
	list := &cobra.Command{Use: "list", Short: "Read payments and summary live from the payments view", RunE: func(cmd *cobra.Command, args []string) error {
		if declarationID == "" {
			return fmt.Errorf("--declaration-id is required")
		}
		session, err := loadSession()
		if err != nil {
			return err
		}
		readback, err := api.NewClient(session).ReadTSDPayments(declarationID)
		if err != nil {
			return err
		}
		return printJSON(readback)
	}}
	list.Flags().StringVar(&declarationID, "declaration-id", "", "Declaration id from tsd list")
	var paymentID string
	del := &cobra.Command{Use: "delete", Short: "Delete one payment row from a TSD draft", RunE: func(cmd *cobra.Command, args []string) error {
		if declarationID == "" || paymentID == "" {
			return fmt.Errorf("--declaration-id and --payment-id are required")
		}
		session, err := loadSession()
		if err != nil {
			return err
		}
		if err := api.NewClient(session).DeleteTSDPayment(declarationID, paymentID); err != nil {
			return err
		}
		return printJSON(map[string]any{"declaration_id": declarationID, "deleted_payment_id": paymentID})
	}}
	del.Flags().StringVar(&declarationID, "declaration-id", "", "Declaration id from tsd list")
	del.Flags().StringVar(&paymentID, "payment-id", "", "Payment id from tsd payments list")
	var fileID string
	feedback := &cobra.Command{Use: "feedback", Short: "Download EMTA validation feedback for an uploaded payments file (id from list.files)", RunE: func(cmd *cobra.Command, args []string) error {
		if fileID == "" {
			return fmt.Errorf("--file-id is required")
		}
		session, err := loadSession()
		if err != nil {
			return err
		}
		data, err := api.NewClient(session).DownloadTSDPaymentsFeedback(fileID)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}}
	feedback.Flags().StringVar(&fileID, "file-id", "", "Uploaded file id from tsd payments list")
	command.AddCommand(upload, list, del, feedback)
	return command
}
