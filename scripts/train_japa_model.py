#!/usr/bin/env python3
"""Train the Japa model — QLoRA SFT on a Llama-family base via Unsloth.

Consumes the chat-format JSONL emitted by cmd/export-traindata:

    {"messages":[{"role":"system",...},{"role":"user",...},{"role":"assistant",...}]}

Trajectory examples additionally carry {"role":"assistant","tool_calls":[...]}
and {"role":"ipython",...} (tool result) turns — rendered natively by the
Llama 3.x chat template.

TEMPLATE SAFETY — before burning GPU hours, verify the base model's chat
template renders tool turns correctly:

    # tokenizer-only check, no CUDA needed:
    python scripts/train_japa_model.py --inspect --train train.jsonl

Training on a rented GPU box (RunPod/Vast.ai/Lambda, ≥24GB VRAM):

    pip install "unsloth[latest]" datasets trl accelerate bitsandbytes
    python scripts/train_japa_model.py \
        --train train.jsonl --eval eval.jsonl --out japa-model

Outputs under --out:
    <out>/lora/          LoRA adapter weights (small, keep for iteration)
    <out>/merged/        base+adapter merged to bf16 (vLLM/serve-ready)
    <out>/gguf/          quantized GGUF for Ollama (q4_k_m default)

Then:

    ollama create japa-model -f Modelfile     # FROM <out>/gguf/*.gguf

The eval split is used only for loss reporting — it never trains.
"""

import argparse
import json
import sys

# Markers that prove the chat template actually rendered tool turns — if a
# rendered trace contains none of these, the template is silently dropping
# or mangling tool_calls/ipython messages and training would teach garbage.
TRACE_MARKERS = ("ipython", "python_tag", "tool_call", "Environment", "tool")


def parse_args():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--train", default="train.jsonl", help="training JSONL from cmd/export-traindata")
    p.add_argument("--eval", default="eval.jsonl", help="held-out eval JSONL (optional)")
    p.add_argument("--base", default="unsloth/Meta-Llama-3.1-8B-Instruct",
                   help="base model — unsloth/ mirrors are pre-quantized for 4bit loading")
    p.add_argument("--out", default="japa-model", help="output directory")
    p.add_argument("--seq-len", type=int, default=4096, help="max training sequence length")
    p.add_argument("--lora-r", type=int, default=16, help="LoRA rank (16 is the sweet spot for 8B SFT)")
    p.add_argument("--epochs", type=float, default=3.0)
    p.add_argument("--lr", type=float, default=2e-4)
    p.add_argument("--batch", type=int, default=2, help="per-device batch size")
    p.add_argument("--accum", type=int, default=8, help="gradient accumulation (effective batch = batch*accum)")
    p.add_argument("--gguf", default="q4_k_m", help="GGUF quantization method (q4_k_m, q8_0, f16...)")
    p.add_argument("--inspect", action="store_true",
                   help="render sample rows through the chat template and exit — no GPU/model load")
    p.add_argument("--force", action="store_true",
                   help="train even if the template check can't confirm tool-turn rendering")
    return p.parse_args()


def load_raw_jsonl(path):
    """Load message lists verbatim — no rendering. Returns [] on missing file."""
    rows = []
    try:
        with open(path) as f:
            for i, line in enumerate(f):
                line = line.strip()
                if not line:
                    continue
                try:
                    rows.append(json.loads(line)["messages"])
                except Exception as e:
                    print(f"  ! {path}:{i+1} skipped ({e})", file=sys.stderr)
    except FileNotFoundError:
        return []
    return rows


def check_template(tokenizer, raw_rows, quiet=False):
    """Render one plain and one trajectory example through the base model's
    chat template and verify tool markers survive. Returns True if the
    trace renders correctly (or no traces exist).

    Catches the failure mode where a template ignores tool_calls keys and
    ipython roles — silently producing malformed training text that teaches
    the model to parrot tools it never learned to call.
    """
    plain = next((m for m in raw_rows if all("tool_calls" not in msg for msg in m)), None)
    trace = next((m for m in raw_rows if any("tool_calls" in msg for msg in m)), None)

    ok = True
    for label, msgs in (("plain", plain), ("trace", trace)):
        if msgs is None:
            continue
        try:
            rendered = tokenizer.apply_chat_template(msgs, tokenize=False)
        except Exception as e:
            # A template that *crashes* on a turn shape (e.g. reading
            # message['content'] on a content-less tool_calls message) is
            # a render failure, not a script bug — report it as such.
            ok = False
            print(f"\n⚠️  {label.upper()} SAMPLE CRASHED in template: {e}", file=sys.stderr)
            continue

        if not quiet:
            print(f"\n{'='*60}\nRENDERED {label.upper()} SAMPLE\n{'='*60}")
            print(rendered[:2500] + ("\n  … [truncated]" if len(rendered) > 2500 else ""))

        if label == "trace":
            tool_names = [tc["function"]["name"]
                          for m in msgs for tc in m.get("tool_calls", [])
                          if "function" in tc]
            names_rendered = all(n in rendered for n in tool_names)
            markers_found = [mk for mk in TRACE_MARKERS if mk in rendered]

            if not names_rendered or not markers_found:
                ok = False
                print(f"\n⚠️  TRACE RENDER FAILED: tool names rendered={names_rendered}, "
                      f"markers found={markers_found or 'NONE'}", file=sys.stderr)
                print("    The base model's chat template does not handle "
                      "tool_calls/ipython turns. Options:", file=sys.stderr)
                print("      - switch --base to a tool-capable instruct model "
                      "(Llama 3.1+ Instruct)", file=sys.stderr)
                print("      - re-export without traces: "
                      "export-traindata -sources behavior,forum,sessions,...", file=sys.stderr)
                print("      - override tokenizer.chat_template before training", file=sys.stderr)
            else:
                print(f"\n✓ trace rendered correctly — markers: {markers_found}, "
                      f"tools: {tool_names}")

    return ok


def load_chat_jsonl(raw_rows, tokenizer):
    """Render each row's messages through the base model's own chat
    template — training must match the template the served model uses."""
    out = []
    for i, msgs in enumerate(raw_rows):
        try:
            out.append({"text": tokenizer.apply_chat_template(msgs, tokenize=False)})
        except Exception as e:
            print(f"  ! row {i+1} skipped ({e})", file=sys.stderr)
    return out


def main():
    args = parse_args()

    raw_train = load_raw_jsonl(args.train)
    if not raw_train:
        sys.exit(f"no usable examples in {args.train}")
    raw_eval = load_raw_jsonl(args.eval) if args.eval else []

    # --inspect: tokenizer-only check, works on any machine (no CUDA).
    if args.inspect:
        from transformers import AutoTokenizer
        print(f"Loading tokenizer for {args.base} …")
        tokenizer = AutoTokenizer.from_pretrained(args.base)
        ok = check_template(tokenizer, raw_train)
        sys.exit(0 if ok else 1)

    # Imported lazily so --inspect/--help work without the CUDA stack.
    import torch
    from datasets import Dataset
    from trl import SFTConfig, SFTTrainer
    from unsloth import FastLanguageModel

    if not torch.cuda.is_available():
        sys.exit("CUDA GPU required — run on a rented GPU box (RunPod/Vast.ai/Lambda)")

    print(f"Base model : {args.base}")
    print(f"Output     : {args.out}")

    model, tokenizer = FastLanguageModel.from_pretrained(
        model_name=args.base,
        max_seq_length=args.seq_len,
        load_in_4bit=True,  # QLoRA: frozen 4-bit base + trainable adapters
    )

    # Template smoke check — cheap on the box, saves a wasted run.
    if not check_template(tokenizer, raw_train, quiet=True):
        if not args.force:
            sys.exit("template check failed — rerun with --inspect to see the "
                     "rendered sample, or --force to train anyway")
        print("⚠️  --force: training despite failed template check", file=sys.stderr)

    model = FastLanguageModel.get_peft_model(
        model,
        r=args.lora_r,
        lora_alpha=args.lora_r,          # alpha=r keeps effective LR stable
        lora_dropout=0,
        target_modules=["q_proj", "k_proj", "v_proj", "o_proj",
                        "gate_proj", "up_proj", "down_proj"],
        use_gradient_checkpointing="unsloth",
    )

    train_rows = load_chat_jsonl(raw_train, tokenizer)
    print(f"Train      : {len(train_rows)} examples")
    if len(train_rows) < 50:
        print("WARNING: <50 examples — a fine-tune this small mostly teaches style, not knowledge",
              file=sys.stderr)

    eval_rows = load_chat_jsonl(raw_eval, tokenizer) if raw_eval else []
    print(f"Eval       : {len(eval_rows)} examples (loss reporting only)" if eval_rows
          else "Eval       : none")

    trainer = SFTTrainer(
        model=model,
        tokenizer=tokenizer,
        train_dataset=Dataset.from_list(train_rows),
        eval_dataset=Dataset.from_list(eval_rows) if eval_rows else None,
        args=SFTConfig(
            output_dir=args.out,
            dataset_text_field="text",
            max_length=args.seq_len,
            packing=False,                 # keep example boundaries clean
            per_device_train_batch_size=args.batch,
            gradient_accumulation_steps=args.accum,
            num_train_epochs=args.epochs,
            learning_rate=args.lr,
            lr_scheduler_type="cosine",
            warmup_ratio=0.05,
            logging_steps=10,
            eval_strategy="steps" if eval_rows else "no",
            eval_steps=50,
            save_strategy="epoch",
            bf16=True,
            report_to="none",              # no wandb on a throwaway GPU box
            seed=42,
        ),
    )

    trainer.train()

    # LoRA adapter — small artifact to keep for iteration/rollback.
    model.save_pretrained(f"{args.out}/lora")
    tokenizer.save_pretrained(f"{args.out}/lora")

    # Merged bf16 weights — serve with vLLM/llama.cpp/TGI or re-quantize.
    model.save_pretrained_merged(f"{args.out}/merged", tokenizer, save_method="merged_16bit")

    # GGUF for Ollama — the path JapaConnect's dispatcher already supports.
    model.save_pretrained_gguf(f"{args.out}/gguf", tokenizer, quantization_method=args.gguf)

    print(f"\nDone. Artifacts under {args.out}/")
    print(f"  ollama create japa-model -f Modelfile   # FROM {args.out}/gguf/*{args.gguf}*.gguf")


if __name__ == "__main__":
    main()
