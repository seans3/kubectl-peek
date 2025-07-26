package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
)

const (
	// DefaultPeekLimit is the default number of items to return per page.
	DefaultPeekLimit int64 = 10
)

// PeekOptions provides the options and dependencies for the peek command.
type PeekOptions struct {
	configFlags *genericclioptions.ConfigFlags
	printFlags  *genericclioptions.PrintFlags

	// User-provided resource type (e.g., "pods", "deployments.apps").
	resource string

	// Flags for the peek command.
	limit         int64
	continueToken string
	interactive   bool
	selector      string
	allNamespaces bool

	// Calculated values.
	namespace     string
	dynamicClient dynamic.Interface
	mapper        meta.RESTMapper

	genericclioptions.IOStreams
}

// NewPeekOptions returns a new instance of PeekOptions with default values.
func NewPeekOptions(streams genericclioptions.IOStreams) *PeekOptions {
	return &PeekOptions{
		configFlags: genericclioptions.NewConfigFlags(true),
		printFlags:  genericclioptions.NewPrintFlags(""), // Type setter is not needed for dynamic client.
		IOStreams:   streams,
	}
}

// NewCmdPeek creates a new cobra command that can be used to run the peek logic.
func NewCmdPeek(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewPeekOptions(streams)

	cmd := &cobra.Command{
		Use:   "peek [type]",
		Short: "Efficiently peek at the first N resources from the API server",
		Long: `The "peek" command allows you to retrieve just the first N items of a resource list,
avoiding the high memory and network usage of "kubectl get" on clusters with many resources.
It supports pagination through an interactive mode or by manually passing a continue token.`,
		Example: `
  # Peek at the first 10 pods in the current namespace
  kubectl peek pods

  # Peek at the first 5 deployments in wide format
  kubectl peek deployments --limit 5 -o wide

  # Interactively page through all services, 20 at a time
  kubectl peek services --limit 20 -i

  # Get the second page of pods, using a token from a previous run
  kubectl peek pods --limit 10 --continue "eyJhbGciOi..."
`,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("you must specify the type of resource to peek")
			}
			if len(args) > 1 {
				return fmt.Errorf("only one resource type is allowed")
			}
			o.resource = args[0]

			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	// Add our custom flags.
	cmd.Flags().Int64Var(&o.limit, "limit", DefaultPeekLimit, "Number of items to return per page.")
	cmd.Flags().StringVar(&o.continueToken, "continue", "", "A token used to retrieve the next page of results. If not provided, the first page is returned.")
	cmd.Flags().BoolVarP(&o.interactive, "interactive", "i", false, "Enable interactive mode to page through results.")
	cmd.Flags().StringVarP(&o.selector, "selector", "l", "", "Selector (label query) to filter on. Supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")

	// Add standard kubectl flags.
	o.configFlags.AddFlags(cmd.Flags())
	o.printFlags.AddFlags(cmd)

	return cmd
}

// Complete sets all information required for processing the command.
func (o *PeekOptions) Complete() error {
	var err error

	// Create a RESTMapper to map resource names (like "pods") to GVRs.
	o.mapper, err = o.configFlags.ToRESTMapper()
	if err != nil {
		return err
	}

	// Get the namespace from the flags.
	o.namespace, _, err = o.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	// Create a dynamic client that can work with any resource type.
	config, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}
	o.dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	return nil
}

// Validate ensures that all required arguments and flag values are provided and valid.
func (o *PeekOptions) Validate() error {
	if o.limit <= 0 {
		return fmt.Errorf("--limit must be a positive number")
	}
	if o.interactive && o.continueToken != "" {
		return fmt.Errorf("cannot use --interactive and --continue flags together")
	}
	// Interactive mode doesn't make sense if the output is not for a human.
	if o.interactive && (*o.printFlags.OutputFormat != "" && *o.printFlags.OutputFormat != "wide") {
		return fmt.Errorf("interactive mode is only supported for standard and wide table output")
	}
	return nil
}

// Run executes the peek command logic.
func (o *PeekOptions) Run() error {
	gvr, err := o.getResourceGVR()
	if err != nil {
		return err
	}

	ns := o.namespace
	if o.allNamespaces {
		ns = "" // An empty string tells the client to query all namespaces.
	}

	continueToken := o.continueToken
	isFirstRequest := true

	for {
		listOptions := metav1.ListOptions{
			Limit:         o.limit,
			Continue:      continueToken,
			LabelSelector: o.selector,
		}

		list, err := o.dynamicClient.Resource(gvr).Namespace(ns).List(context.Background(), listOptions)
		if err != nil {
			return err
		}

		// If it's the first page and there are no items, just say so and exit.
		if isFirstRequest && len(list.Items) == 0 {
			fmt.Fprintln(o.Out, "No resources found.")
			return nil
		}

		// Use the standard kubectl printers to format the output.
		printer, err := o.printFlags.ToPrinter()
		if err != nil {
			return err
		}
		if err := printer.PrintObj(list, o.Out); err != nil {
			return err
		}

		isFirstRequest = false
		continueToken = list.GetContinue()

		// If there's no token, we've reached the end of the list.
		if continueToken == "" {
			if o.interactive {
				fmt.Fprintln(o.Out, "\n--- End of list ---")
			}
			return nil
		}

		// Handle pagination flow.
		if o.interactive {
			fmt.Fprintf(o.Out, "\n--- [n] next page, [q] quit: ")
			reader := bufio.NewReader(os.Stdin)
			char, _, err := reader.ReadRune()
			if err != nil {
				return err
			}
			fmt.Println() // Newline for clean formatting after user input.
			if char != 'n' {
				return nil // Quit on any key other than 'n'.
			}
		} else {
			// In non-interactive mode, print the token and exit.
			fmt.Fprintf(o.Out, "\nContinue Token: %s\n", continueToken)
			return nil
		}
	}
}

// getResourceGVR finds the GroupVersionResource for a given short resource name.
func (o *PeekOptions) getResourceGVR() (schema.GroupVersionResource, error) {
	resourceArg := strings.ToLower(o.resource)

	// Create a partial GVR from the user's argument. We don't know the version,
	// so we leave it empty. The RESTMapper will find the best match.
	// This approach handles "pods", "deployments", and "deployments.apps" style arguments.
	gvrToFind := schema.GroupVersionResource{}
	parts := strings.Split(resourceArg, ".")
	if len(parts) == 2 {
		gvrToFind = schema.GroupVersionResource{Group: parts[1], Resource: parts[0]}
	} else {
		gvrToFind = schema.GroupVersionResource{Resource: resourceArg}
	}

	gvr, err := o.mapper.ResourceFor(gvrToFind)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("the server doesn't have a resource type %q", o.resource)
	}

	return gvr, nil
}

// main is the entrypoint for the kubectl-peek plugin.
func main() {
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewCmdPeek(streams)

	// To make this a valid kubectl plugin, the binary must be named "kubectl-peek".
	// We can use cobra's utility to set the command path for help messages,
	// which makes it look like a native command.
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
