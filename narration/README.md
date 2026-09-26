# Narration library

Import `github.com/narrate-it/narrate/narration` to reuse the same embedded
craft modules, chunking, script validation and rewriting as the Narrate CLI.
This is a facade over the existing implementation; prompt assets are not copied.

`Assemble("conversational")`, `Assemble("coach")` and
`Assemble("agent-update")` return the canonical instructions.
`EffectiveDigest` and `Manifest` expose asset provenance for cache identity
and evidence. Keep source content separate from privileged instructions.

For rewriting, construct `Rewriter` with a host-supplied `RewriteClient`.
The host owns model choice, credentials, network transport and accounting.
Set `DisableRetries: true` when each physical request needs separate admission;
this disables both transient retries and script repair calls. The CLI retains
its existing retry behavior by default.

The package does not read environment variables, discover API keys, launch
processes, play audio or select a provider. Speech recognition and synthesis
are separate capabilities; these craft instructions do not implement them.
