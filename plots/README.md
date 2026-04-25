# Plotting Directory

This directory contains everything needed to generate plots about Stinglet after the experiments have been run. It has the following subfolders:

- [`data/`](data/) — CSV files used as input by the plotting scripts.
- [`scripts/`](scripts/) — Python scripts that read from `data/` and generate plots.
- [`figures/`](figures/) — Output PDF figures produced by the scripts. These are the figures to include in the paper.

This directory does NOT contain scripts to run the experiments (i.e. the scripts that produce the files in [`data/`](data/)). The user is expected to run the experiments separately, and then manually copy the files produced by the experiments to [`data/`](data/).

## Setup

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Generating the plots

Run all scripts from the `plots/` directory:

```bash
python scripts/<your_script>.py
```

⚠️ To know which figure from the HCDS paper a script generates, open the script
and read the comment at the top of it (after the imports), which states the
figure number.
