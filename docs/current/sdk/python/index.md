---
slug: /sdk/python
---

# Dagger Python SDK

## What is the Dagger Python SDK?

The Dagger Python SDK contains everything you need to develop CI/CD pipelines in Python, and run them on any OCI-compatible container runtime.

Currently the Python SDK consists of:

* A Python package
* This documentation

## Who is it for?

The Dagger Python SDK may be a good fit if you are...

* A Python developer wishing your CI pipelines were Python code instead of YAML.
* A developer who needs CI/CD, and is looking for an excuse to learn Python.
* Your team's "designated devops person", hoping to replace a pile of artisanal scripts with something more powerful.
* A platform engineer writing custom Python tooling, with the goal of unifying continuous delivery across organizational silos.
* A data engineer looking to better integrate with your organization's CI/CD or MLOps pipelines.

The Dagger Python SDK may *not* be a good fit if you are...

* A developer who doesn't know Python, and is not interested in learning it.
* Someone who loves writing YAML all day, thank you very much.
* A container skeptic: the less containers are involved, the happier you are.

## How does it work?

```mermaid
graph LR;

subgraph program["Your Python program"]
  lib["Python package"]
end

engine["Dagger Engine"]
oci["OCI container runtime"]

subgraph A["your build pipeline"]
  A1[" "] -.-> A2[" "] -.-> A3[" "]
end
subgraph B["your test pipeline"]
  B1[" "] -.-> B2[" "] -.-> B3[" "] -.-> B4[" "]
end
subgraph C["your deployment pipeline"]
  C1[" "] -.-> C2[" "] -.-> C3[" "] -.-> C4[" "]
end

lib -..-> engine -..-> oci -..-> A1 & B1 & C1
```

1. Your Python program imports the Dagger Python package.
2. Using the Python package, your program opens a new session to a Dagger Engine: either by connecting to an existing engine, or by provisioning one on-the-fly.
3. Using the Python package, your program prepares API requests describing pipelines to run, then sends them to the engine. The wire protocol used to communicate with the engine is private and not yet documented, but this will change in the future. For now, the Python package is the only documented API available to your program.
4. When the engine receives an API request, it computes a [Directed Acyclic Graph (DAG)](https://en.wikipedia.org/wiki/Directed_acyclic_graph) of low-level operations required to compute the result, and starts processing operations concurrently.
5. When all operations in the pipeline have been resolved, the engine sends the pipeline result back to your program.
6. Your program may use the pipeline's result as input to new pipelines.

## Get started

To learn more, [install the Python SDK](./866944-install.md) and [start using it](./628797-get-started.md).
