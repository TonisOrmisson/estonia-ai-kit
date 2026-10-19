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
	var confirm bool
	var reportType string
	var downloadHref string

	kmdCmd := &cobra.Command{
		Use:   "kmd",
		Short: "Käibedeklaratsioon (KMD) operations",
	}
	idFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&declarationID, "declaration-id", "", "Declaration id from kmd list")
	}
	inputFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&inputPath, "input", "", "Path to KMD declaration JSON (tax_payer_reg_code, year, month, main, inf_a, inf_b)")
	}
	need := func(pairs ...string) error {
		for i := 0; i < len(pairs); i += 2 {
			if pairs[i+1] == "" {
				return fmt.Errorf("%s is required", pairs[i])
			}
		}
		return nil
	}

	listCmd := &cobra.Command{
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

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create an empty KMD draft for a period",
		RunE: func(cmd *cobra.Command, args []string) error {
			if year == 0 || month == 0 {
				return fmt.Errorf("--year and --month are required")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			if _, err := client.CreateKMDDraft(year, month); err != nil {
				return err
			}
			items, err := client.ListKMDDeclarations()
			if err != nil {
				return err
			}
			id := findDraftDeclarationID(items, year, month)
			if id == "" {
				return fmt.Errorf("created draft for %04d-%02d but could not resolve declaration id from list", year, month)
			}
			return printJSON(map[string]string{"declaration_id": id})
		},
	}
	createCmd.Flags().IntVar(&year, "year", 0, "Tax year")
	createCmd.Flags().IntVar(&month, "month", 0, "Tax month (1-12)")

	xmlCmd := &cobra.Command{
		Use:   "xml",
		Short: "KMD XML (official vatdeclaration.xsd) build and import",
	}

	xmlBuildCmd := &cobra.Command{
		Use:   "build",
		Short: "Validate declaration JSON and render KMD XML (offline)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--input", inputPath); err != nil {
				return err
			}
			in, err := readKMDInput(inputPath)
			if err != nil {
				return err
			}
			xmlBytes, err := api.BuildKMDXML(in)
			if err != nil {
				return err
			}
			if outputPath == "" {
				_, err = os.Stdout.Write(xmlBytes)
				return err
			}
			return os.WriteFile(outputPath, xmlBytes, 0o600)
		},
	}
	inputFlag(xmlBuildCmd)
	xmlBuildCmd.Flags().StringVar(&outputPath, "output", "", "Write XML here instead of stdout")

	xmlImportCmd := &cobra.Command{
		Use:   "import",
		Short: "Upload declaration as KMD XML into a draft, check EMTA upload errors, then verify live",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID, "--input", inputPath); err != nil {
				return err
			}
			in, err := readKMDInput(inputPath)
			if err != nil {
				return err
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			imported, err := client.ImportKMDXML(declarationID, in)
			if err != nil {
				return err
			}
			verified, err := client.VerifyKMD(declarationID, in)
			if err != nil {
				return fmt.Errorf("uploaded %s but live verify failed: %w", imported.RawRedirect, err)
			}
			if err := printJSON(map[string]any{"uploaded_file": imported.RawRedirect, "messages": imported.Messages, "verify": verified}); err != nil {
				return err
			}
			if !verified.OK {
				return fmt.Errorf("live EMTA draft does not match input: %d difference(s)", len(verified.Differences))
			}
			return nil
		},
	}
	idFlag(xmlImportCmd)
	inputFlag(xmlImportCmd)

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Read the draft live from EMTA and compare it with the declaration JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID, "--input", inputPath); err != nil {
				return err
			}
			in, err := readKMDInput(inputPath)
			if err != nil {
				return err
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			verified, err := client.VerifyKMD(declarationID, in)
			if err != nil {
				return err
			}
			if err := printJSON(verified); err != nil {
				return err
			}
			if !verified.OK {
				return fmt.Errorf("live EMTA draft does not match input: %d difference(s)", len(verified.Differences))
			}
			return nil
		},
	}
	idFlag(verifyCmd)
	inputFlag(verifyCmd)

	uploadErrorsCmd := &cobra.Command{
		Use:   "upload-errors",
		Short: "List EMTA file upload errors shown on the declaration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID); err != nil {
				return err
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			rows, err := client.ReadKMDUploadErrors(declarationID)
			if err != nil {
				return err
			}
			return printJSON(rows)
		},
	}
	idFlag(uploadErrorsCmd)

	submitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a KMD draft (requires EMTA_ALLOW_SUBMIT=1, --confirm and a passing live verify)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID, "--input", inputPath); err != nil {
				return err
			}
			if err := requireSubmitPermission(confirm); err != nil {
				return err
			}
			in, err := readKMDInput(inputPath)
			if err != nil {
				return err
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			verified, err := client.VerifyKMD(declarationID, in)
			if err != nil {
				return err
			}
			if !verified.OK {
				_ = printJSON(verified)
				return fmt.Errorf("refusing to submit: live draft does not match input")
			}
			result, err := client.SubmitKMD(declarationID)
			if err != nil {
				return err
			}
			return printJSON(result)
		},
	}
	idFlag(submitCmd)
	inputFlag(submitCmd)
	submitCmd.Flags().BoolVar(&confirm, "confirm", false, "Actually submit the declaration")

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an unsent KMD draft by declaration id",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID); err != nil {
				return err
			}
			if !confirm {
				return fmt.Errorf("--confirm is required for delete")
			}
			client, err := loadEMTAClient()
			if err != nil {
				return err
			}
			return client.DeleteKMDDraft(declarationID)
		},
	}
	idFlag(deleteCmd)
	deleteCmd.Flags().BoolVar(&confirm, "confirm", false, "Actually delete the draft")

	readCmd := func(use, short string, read func(*api.Client, string) (any, error)) *cobra.Command {
		c := &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := need("--declaration-id", declarationID); err != nil {
					return err
				}
				client, err := loadEMTAClient()
				if err != nil {
					return err
				}
				v, err := read(client, declarationID)
				if err != nil {
					return err
				}
				return printJSON(v)
			},
		}
		idFlag(c)
		return c
	}
	mainCmd := &cobra.Command{Use: "main", Short: "KMD base form"}
	mainCmd.AddCommand(readCmd("read", "Read KMD main form live from EMTA", func(c *api.Client, id string) (any, error) { return c.ReadKMDMain(id) }))
	infACmd := &cobra.Command{Use: "inf-a", Short: "KMD INF A section"}
	infACmd.AddCommand(readCmd("read", "Read KMD INF A rows live from EMTA", func(c *api.Client, id string) (any, error) { return c.ReadKMDINFA(id) }))
	infBCmd := &cobra.Command{Use: "inf-b", Short: "KMD INF B section"}
	infBCmd.AddCommand(readCmd("read", "Read KMD INF B rows live from EMTA", func(c *api.Client, id string) (any, error) { return c.ReadKMDINFB(id) }))

	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "KMD generated export files (Genereeri fail)",
	}

	fileRequestCmd := &cobra.Command{
		Use:   "request",
		Short: "Request generation of a KMD export file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID, "--report-type", reportType); err != nil {
				return err
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
	idFlag(fileRequestCmd)
	fileRequestCmd.Flags().StringVar(&reportType, "report-type", "", "One of: main, inf-a, inf-a-summary, inf-b, inf-b-summary, all")

	fileListCmd := readCmd("list-generated", "List generated KMD files for a declaration", func(c *api.Client, id string) (any, error) {
		page, err := c.OpenKMDGeneratedFilesPage(id)
		if err != nil {
			return nil, err
		}
		return api.ParseKMDGeneratedFilesForCLI(page.HTML)
	})

	fileDownloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Download a generated KMD file by href",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := need("--declaration-id", declarationID, "--href", downloadHref, "--output", outputPath); err != nil {
				return err
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
			return printJSON(map[string]string{"output": outputPath, "file_name": result.FileName})
		},
	}
	idFlag(fileDownloadCmd)
	fileDownloadCmd.Flags().StringVar(&downloadHref, "href", "", "Download href from list-generated output")
	fileDownloadCmd.Flags().StringVar(&outputPath, "output", "", "Path to save generated file")

	xmlCmd.AddCommand(xmlBuildCmd, xmlImportCmd)
	fileCmd.AddCommand(fileRequestCmd, fileListCmd, fileDownloadCmd)
	kmdCmd.AddCommand(listCmd, createCmd, xmlCmd, verifyCmd, uploadErrorsCmd, submitCmd, deleteCmd, mainCmd, infACmd, infBCmd, fileCmd)
	rootCmd.AddCommand(kmdCmd)
}

// requireSubmitPermission keeps declaration submission an explicit human decision:
// agents and scripts cannot submit unless the operator exported EMTA_ALLOW_SUBMIT=1.
func requireSubmitPermission(confirm bool) error {
	if os.Getenv("EMTA_ALLOW_SUBMIT") != "1" {
		return fmt.Errorf("submit is disabled: set EMTA_ALLOW_SUBMIT=1 in your own shell to allow it")
	}
	if !confirm {
		return fmt.Errorf("--confirm is required for submit")
	}
	return nil
}

func readKMDInput(path string) (*api.KMDDeclarationInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return api.ParseKMDDeclarationInput(data)
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

func findDraftDeclarationID(items []api.KMDListItem, year, month int) string {
	for _, item := range items {
		if item.Year == year && item.Month == month && item.DeclarationID != "" && !strings.EqualFold(item.Status, "Esitatud") {
			return item.DeclarationID
		}
	}
	for _, item := range items {
		if item.Year == year && item.Month == month && item.DeclarationID != "" {
			return item.DeclarationID
		}
	}
	return ""
}
