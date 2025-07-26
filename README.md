# kubectl-peek: A `kubectl` Plugin for Efficiently Viewing Large Resource Lists

`kubectl-peek` is a `kubectl` plugin designed to efficiently "peek" at the first `N` items from a resource list returned by the Kubernetes API server.

The primary problem this plugin solves is the inefficiency of `kubectl get` when dealing with a massive number of resources (e.g., 100,000+ pods). Standard `kubectl get` must fetch the entire list of items from the API server before it can filter, format, and display them. This can lead to significant memory consumption on the client-side, high network bandwidth usage, and slow performance, sometimes even causing the client to hang or crash.

`kubectl-peek` solves this by retrieving only `N` items at a time directly from the API server, providing a fast and lightweight way to inspect subsets of resources in a crowded cluster.

-----

## 🚀 Features

  * **Efficient Fetching**: Uses the API server's built-in `limit` parameter to retrieve only the number of items you ask for.
  * **Stateless Manual Pagination**: Natively supports the API server's pagination mechanism using `continue` tokens for scriptable, page-by-page Browse.
  * **Interactive Mode**: Provides a simple, interactive interface to seamlessly page through results with single key presses.
  * **Familiar `get` Flags**: Implements most of the flags you already know from `kubectl get`, such as `-n` (namespace), `-l` (label selector), and all output formats (`-o wide`, `-o yaml`, etc.).
  * **Lightweight**: Avoids high memory and CPU usage on your local machine by not processing the entire resource list.

### Unsupported `get` Flags

Certain flags from `kubectl get` are not supported because they require the entire list of items to function correctly. The main unsupported flag is:

  * **`--sort-by`**: Sorting requires having the full collection of items to determine their correct order. Since `peek` only fetches a subset, client-side sorting would be misleading.

-----

## 🛠️ Installation


To install, you must build the plugin from the source code.

### Prerequisites

* Go (version 1.18 or higher)

* A working `kubectl` and a configured Kubernetes cluster

### Build Steps

**Get the Source Code**
Clone the repository to your local machine.

```bash
git clone https://github.com/seans3/kubectl-peek.git

**Download Dependencies**
This command will find and download all the necessary libraries defined in the `go.mod` file.

```bash
go mod tidy
```

**Build the Binary**
The output binary name is critical. For `kubectl` to recognize it as a plugin, it must be named `kubectl-peek`.

```bash
go build -o kubectl-peek .
```

**Make the Binary Executable**

```bash
chmod +x kubectl-peek
```

**Move the Binary to Your PATH**
For `kubectl` to discover the plugin, the executable must be in a directory listed in your system's `$PATH`. A common location is `/usr/local/bin`.

```bash
sudo mv kubectl-peek /usr/local/bin/
```

**Verify the Installation**
Check if `kubectl` lists your new plugin. You should see `peek` in the list of available commands.

```bash
kubectl
```

-----

## 💻 Usage

The basic usage is `kubectl peek <resource-type> --limit <N>`, where `N` is the number of items you want to see per page.

### Interactive Mode

For the best user experience, use the **`--interactive`** (or **`-i`**) flag. This lets you page through results without manually handling tokens.

```bash
kubectl peek pods --limit 5 --interactive
```

This will display the first 5 pods and wait for your input.

```text
# Initial output
NAME     READY   STATUS    RESTARTS   AGE
pod-a    1/1     Running   0          2d
pod-b    1/1     Running   0          2d
pod-c    1/1     Running   0          2d
pod-d    1/1     Running   0          2d
pod-e    1/1     Running   0          1d

---
[n] next page, [q] quit: # Press 'n' to see the next 5 pods
```

### Manual Pagination

If the list of resources is larger than the specified limit, `kubectl-peek` will print a `continue` token. You can use this token in scripts or to manually fetch the next page of results.

  * **Step 1: Fetch the first page of pods.**

    ```bash
    $ kubectl peek pods --limit 3
    NAME     READY   STATUS    RESTARTS   AGE
    pod-a    1/1     Running   0          2d
    pod-b    1/1     Running   0          2d
    pod-c    1/1     Running   0          2d

    Continue Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
    ```

  * **Step 2: Use the token to fetch the next page.**

    ```bash
    $ kubectl peek pods --limit 3 --continue "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    NAME     READY   STATUS    RESTARTS   AGE
    pod-d    1/1     Running   0          2d
    pod-e    1/1     Running   0          1d
    pod-f    1/1     Running   0          1d
    ```

-----

## 🏗️ Design and Implementation

The plugin is written in **Go** and leverages the official Kubernetes `client-go` and `cli-runtime` libraries to ensure seamless integration and a consistent user experience.

### Core Logic: API Server Limiting and Pagination

`kubectl-peek`'s efficiency comes from the combination of the `limit` and `continue` fields in the `metav1.ListOptions` struct.

1.  **Limiting**: The `Limit` parameter tells the API server to only return a specific number of items.
2.  **Pagination**: If more items exist, the API server's response includes a `continue` token. Our plugin uses this token for subsequent requests to get the next page.

### Interactive Mode

When the `--interactive` flag is used, the plugin enters a loop after fetching the first page. It uses a Go library to read raw keyboard input, allowing it to respond to single key presses without requiring the user to press Enter.

The interactive flow is:

1.  Fetch and display a page of results.
2.  Store the `continue` token from the response.
3.  If no token is returned, it means we've reached the end of the list, and the plugin exits.
4.  If a token is present, prompt the user with navigation options (e.g., `[n] next, [q] quit`).
5.  Wait for a keypress. If the user presses **`n`**, the plugin uses the stored token to fetch the next page and repeats the process. If the user presses **`q`**, the plugin exits.

-----

## 🤔 Why Not Just Use `head`?

A common question is, "Why not just do this?"

```bash
# This is INEFFICIENT!
kubectl get pods -o name | head -n 10
```

The difference is **where the filtering happens**.

  * **`kubectl get ... | head`**: `kubectl` fetches **ALL** pods from the API server, which can be thousands of items. Your machine then processes that massive list, and `head` just shows the first few lines. This is slow and memory-intensive.
  * **`kubectl peek`**: `kubectl-peek` tells the API server, "Please only give me 10 pods." The server sends back a tiny response. This is extremely efficient in terms of network, memory, and CPU.

-----

## 🔮 Future Enhancements

  * **Backward Pagination**: Add support for a "previous page" (`p`) option in interactive mode. This would require caching previous results, as the Kubernetes API only supports forward pagination.
  * **Resource Watching**: Support for `peek --watch`, which would function like `get --watch` but potentially with server-side limits on the initial list returned.


