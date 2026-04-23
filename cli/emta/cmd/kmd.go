package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stefanoamorelli/estonia-ai-kit/cli/emta/api"
)

func init() {
	var declarationID string
	var inputPath string
	var outputPath string
	var year int
	var month int
	var submitConfirm bool
	var deleteConfirm bool
	var partnerCode string
	var invoiceNumber string
	var reportType string
	var downloadHref string

	kmdCmd := &cobra.Command{
		Use:   "kmd",
		Short: "Käibedeklaratsioon (KMD) operations",
	}

	kmdFileCmd := &cobra.Command{
		Use:   "file",
		Short: "KMD file import/export operations",
	}

	kmdListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all KMD declarations",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			items, err := client.ListKMDDeclarations()
			if err != nil {
				return err
			}
			return printJSON(items)
		},
	}

	kmdSubmitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a saved KMD draft by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			if !submitConfirm {
				return fmt.Errorf("--confirm is required for submit")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			result, err := client.SubmitKMD(declarationID)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	kmdSubmitCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	kmdSubmitCmd.Flags().BoolVar(&submitConfirm, "confirm", false, "Actually submit the declaration")

	kmdDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an unsent KMD draft by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			if !deleteConfirm {
				return fmt.Errorf("--confirm is required for delete")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			return client.DeleteKMDDraft(declarationID)
		},
	}
	kmdDeleteCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	kmdDeleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Actually delete the draft")

	kmdFileImportCmd := &cobra.Command{
		Use:   "import",
		Short: "Import a file into an existing KMD draft",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || inputPath == "" {
				return fmt.Errorf("--declaration-id and --input are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			fileBytes, err := os.ReadFile(inputPath)
			if err != nil {
				return err
			}
			result, err := client.ImportKMDFile(declarationID, inputPath, fileBytes)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	kmdFileImportCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	kmdFileImportCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD input file")

	kmdFileCreateCmd := &cobra.Command{
		Use:   "create-from-file",
		Short: "Create a new KMD draft and import file data into it",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputPath == "" || year == 0 || month == 0 {
				return fmt.Errorf("--input, --year and --month are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			fileBytes, err := os.ReadFile(inputPath)
			if err != nil {
				return err
			}
			result, err := client.CreateKMDDraftFromFile(year, month, inputPath, fileBytes)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	kmdFileCreateCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD input file")
	kmdFileCreateCmd.Flags().IntVar(&year, "year", 0, "Tax year")
	kmdFileCreateCmd.Flags().IntVar(&month, "month", 0, "Tax month (1-12)")

	kmdFileRequestCmd := &cobra.Command{
		Use:   "request",
		Short: "Request generation of a KMD export file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || reportType == "" {
				return fmt.Errorf("--declaration-id and --report-type are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			rows, err := client.RequestKMDGeneratedFile(declarationID, reportType)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	kmdFileRequestCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	kmdFileRequestCmd.Flags().StringVar(&reportType, "report-type", "", "One of: main, inf-a, inf-a-summary, inf-b, inf-b-summary, all")

	kmdFileListCmd := &cobra.Command{
		Use:   "list-generated",
		Short: "List generated KMD files for a declaration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			page, err := client.OpenKMDGeneratedFilesPage(declarationID)
			if err != nil {
				return err
			}
			rows, err := api.ParseKMDGeneratedFilesForCLI(page.HTML)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	kmdFileListCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")

	kmdFileDownloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Download a generated KMD file by href",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || downloadHref == "" || outputPath == "" {
				return fmt.Errorf("--declaration-id, --href and --output are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			page, err := client.OpenKMDGeneratedFilesPage(declarationID)
			if err != nil {
				return err
			}
			result, err := client.DownloadKMDGeneratedFile(page.PageURL, downloadHref)
			if err != nil {
				return err
			}
			if err := os.WriteFile(outputPath, result.Bytes, 0o600); err != nil {
				return err
			}
			return printJSON(map[string]string{
				"output":    outputPath,
				"file_name": result.FileName,
			})
		},
	}
	kmdFileDownloadCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	kmdFileDownloadCmd.Flags().StringVar(&downloadHref, "href", "", "Download href from list-generated output")
	kmdFileDownloadCmd.Flags().StringVar(&outputPath, "output", "", "Path to save generated file")

	mainCmd := &cobra.Command{
		Use:   "main",
		Short: "KMD base form",
	}

	mainCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new KMD draft and optionally fill the main form",
		RunE: func(cmd *cobra.Command, args []string) error {
			if year == 0 || month == 0 {
				return fmt.Errorf("--year and --month are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}

			section, err := client.CreateKMDDraft(year, month)
			if err != nil {
				return err
			}

			if inputPath != "" {
				var patch api.KMDMainPatch
				if err := readJSONFile(inputPath, &patch); err != nil {
					return err
				}

				items, err := client.ListKMDDeclarations()
				if err != nil {
					return err
				}
				declarationID = findDraftDeclarationID(items, year, month)
				if declarationID == "" {
					return fmt.Errorf("created draft for %04d-%02d but could not resolve declaration id from list", year, month)
				}

				section, err = client.UpdateKMDMainFromPatch(declarationID, patch)
				if err != nil {
					return err
				}
			}

			if section.DeclarationID == "" {
				items, err := client.ListKMDDeclarations()
				if err == nil {
					section.DeclarationID = findDraftDeclarationID(items, year, month)
				}
			}

			return printJSON(section)
		},
	}
	mainCreateCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD main JSON input")
	mainCreateCmd.Flags().IntVar(&year, "year", 0, "Tax year")
	mainCreateCmd.Flags().IntVar(&month, "month", 0, "Tax month (1-12)")

	mainReadCmd := &cobra.Command{
		Use:   "read",
		Short: "Read KMD main form by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			exported, err := client.ExportKMDReport(declarationID, "main")
			if err != nil {
				section, readErr := client.ReadKMDMain(declarationID)
				if readErr != nil {
					return err
				}
				return printJSON(section)
			}
			section, err := api.ParseKMDMainCSV(exported.Bytes)
			if err != nil {
				return err
			}
			section.DeclarationID = declarationID
			return printJSON(section)
		},
	}
	mainReadCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")

	mainUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update and save KMD main form by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || inputPath == "" {
				return fmt.Errorf("--declaration-id and --input are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			var patch api.KMDMainPatch
			if err := readJSONFile(inputPath, &patch); err != nil {
				return err
			}
			section, err := client.UpdateKMDMainFromPatch(declarationID, patch)
			if err != nil {
				return err
			}
			return printJSON(section)
		},
	}
	mainUpdateCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")
	mainUpdateCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD main JSON input")

	infACmd := &cobra.Command{
		Use:   "inf-a",
		Short: "KMD INF A section",
	}

	infAReadCmd := &cobra.Command{
		Use:   "read",
		Short: "Read KMD INF A rows by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			exported, err := client.ExportKMDReport(declarationID, "inf-a")
			if err != nil {
				rows, readErr := client.ReadKMDINFA(declarationID)
				if readErr != nil {
					return err
				}
				return printJSON(rows)
			}
			rows, err := api.ParseKMDINFACSV(exported.Bytes)
			if err != nil {
				return err
			}
			rows.DeclarationID = declarationID
			return printJSON(rows)
		},
	}
	infAReadCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")

	infAUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "Add/update KMD INF A rows by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || inputPath == "" {
				return fmt.Errorf("--declaration-id and --input are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			var patch api.KMDINFAPatch
			if err := readJSONFile(inputPath, &patch); err != nil {
				return err
			}
			rows, err := client.UpdateKMDINFAFromPatch(declarationID, patch)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	infAUpdateCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")
	infAUpdateCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD INF A JSON input")

	infADeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete KMD INF A row by partner code and invoice number",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || partnerCode == "" || invoiceNumber == "" {
				return fmt.Errorf("--declaration-id, --partner-code and --invoice-number are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			rows, err := client.DeleteKMDINFAFromFile(declarationID, partnerCode, invoiceNumber)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	infADeleteCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	infADeleteCmd.Flags().StringVar(&partnerCode, "partner-code", "", "Partner code")
	infADeleteCmd.Flags().StringVar(&invoiceNumber, "invoice-number", "", "Invoice number")

	infBCmd := &cobra.Command{
		Use:   "inf-b",
		Short: "KMD INF B section",
	}

	infBReadCmd := &cobra.Command{
		Use:   "read",
		Short: "Read KMD INF B rows by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" {
				return fmt.Errorf("--declaration-id is required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			exported, err := client.ExportKMDReport(declarationID, "inf-b")
			if err != nil {
				rows, readErr := client.ReadKMDINFB(declarationID)
				if readErr != nil {
					return err
				}
				return printJSON(rows)
			}
			rows, err := api.ParseKMDINFBCSV(exported.Bytes)
			if err != nil {
				return err
			}
			rows.DeclarationID = declarationID
			return printJSON(rows)
		},
	}
	infBReadCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")

	infBUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "Add/update KMD INF B rows by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || inputPath == "" {
				return fmt.Errorf("--declaration-id and --input are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			var patch api.KMDINFBPatch
			if err := readJSONFile(inputPath, &patch); err != nil {
				return err
			}
			rows, err := client.UpdateKMDINFBFromPatch(declarationID, patch)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	infBUpdateCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Opaque declaration id from kmd list")
	infBUpdateCmd.Flags().StringVar(&inputPath, "input", "", "Path to KMD INF B JSON input")

	infBDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete KMD INF B row by partner code and invoice number",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || partnerCode == "" || invoiceNumber == "" {
				return fmt.Errorf("--declaration-id, --partner-code and --invoice-number are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			rows, err := client.DeleteKMDINFBFromFile(declarationID, partnerCode, invoiceNumber)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	infBDeleteCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from kmd list")
	infBDeleteCmd.Flags().StringVar(&partnerCode, "partner-code", "", "Partner code")
	infBDeleteCmd.Flags().StringVar(&invoiceNumber, "invoice-number", "", "Invoice number")

	mainCmd.AddCommand(mainCreateCmd, mainReadCmd, mainUpdateCmd)
	infACmd.AddCommand(infAReadCmd, infAUpdateCmd, infADeleteCmd)
	infBCmd.AddCommand(infBReadCmd, infBUpdateCmd, infBDeleteCmd)
	kmdFileCmd.AddCommand(kmdFileImportCmd, kmdFileCreateCmd, kmdFileRequestCmd, kmdFileListCmd, kmdFileDownloadCmd)
	kmdCmd.AddCommand(kmdListCmd, kmdSubmitCmd, kmdDeleteCmd, kmdFileCmd, mainCmd, infACmd, infBCmd)
	rootCmd.AddCommand(kmdCmd)
}

func loadEMTAClient() (*api.Client, error) {
	session, err := loadSession()
	if err != nil {
		return nil, err
	}
	return api.NewClient(session), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

func findDraftDeclarationID(items []api.KMDListItem, year, month int) string {
	for _, item := range items {
		if item.Year == year && item.Month == month && item.UpdateID != "" && !strings.EqualFold(item.Status, "Esitatud") {
			return item.DeclarationID
		}
	}
	for _, item := range items {
		if item.Year == year && item.Month == month && item.UpdateID != "" {
			return item.DeclarationID
		}
	}
	return ""
}
