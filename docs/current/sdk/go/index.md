---
slug: /sdk/go
---

# Dagger Go SDK

## What is the Dagger Go SDK?

The Dagger Go SDK contains everything you need to develop CI/CD pipelines in Go, and run them with the [Dagger Engine][dagger].

This SDK contains:

* A Go package: `dagger.io/dagger`
* The [`dagger` CLI][cli]
* [Examples][examples]
* [Documentation][docs]

## Who is it for?

The Dagger Go SDK may be a good fit if you are...

* A Go developer wishing your CI pipelines were Go code instead of YAML.
* A developer who needs CI/CD, and is looking for an excuse to learn Go.
* Your team's "designated devops person", hoping to replace a pile of artisanal scripts with something more powerful.
* A platform engineer writing custom Go tooling, with the goal of unifying continuous delivery across organizational silos.
* A cloud-native developer advocate or solutions engineer, looking to demonstrate a complex integration on short notice.

The Dagger Go SDK may *not* be a good fit if you are...

* A developer who doesn't know Go, and is not interested in learning it.
* A "designated devops person" who doesn't think of themselves as a developer - nothing makes you happier than a battle-hardened shell script.
* Someone who loves writing YAML all day, thank you very much.
* A container skeptic: the less containers are involved, the happier you are.

## How does it work?

```mermaid
graph LR;

subgraph program["Your Go program"]
  lib["Go library"]
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

1. Your Go program imports the Dagger Go library.
2. Using the Go library, your program opens a new session to a Dagger Engine: either by connecting to an existing engine, or by provisioning one on-the-fly.
3. Using the Go library, your program prepares API requests describing pipelines to run, then sends them to the engine. The wire protocol used to communicate with the engine is private and not yet documented, but this will change in the future. For now, the Go library is the only documented API available to your program.
4. When the engine receives an API request, it computes a [Directed Acyclic Graph (DAG)][dag] of low-level operations required to compute the result, and starts processing operations concurrently.
5. When all operations in the pipeline have been resolved, the engine sends the pipeline result back to your program.
6. Your program may use the pipeline's result as input to new pipelines.

## Get started

To learn more, [install the Go SDK][install] and [start using it][get-started].

[dagger]: https://dagger.io
[cli]: https://docs.dagger.io/cli
[examples]: https://github.com/dagger/examples/tree/main/go
[docs]: https://docs.dagger.io/sdk/go
[install]: ./371491-install.md
[get-started]: ./959738-get-started.md
[dag]: https://en.wikipedia.org/wiki/Directed_acyclic_graph
