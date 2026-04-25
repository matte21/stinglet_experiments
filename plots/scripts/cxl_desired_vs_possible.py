import matplotlib as mpl
import matplotlib.pyplot as plt
import pandas as pd
import numpy as np

mpl.rcParams['pdf.fonttype'] = 42
mpl.rcParams['ps.fonttype'] = 42

# Configure for academic paper style
plt.rcParams.update({
    'font.size': 12,
    'font.family': 'serif',
    'axes.linewidth': 1,
    'grid.linewidth': 0.5,
    'grid.alpha': 0.3,
    'figure.dpi': 300
})

# Read CSV, skipping empty rows
df = pd.read_csv('data/cxl_desired_vs_possible.csv', skiprows=2, index_col=0)

# Set up the plot
fig, ax = plt.subplots(figsize=(7, 3))

# Define colors
colors = ['#4472C4', '#E15759']

# Create grouped bar chart with spacing
x = np.arange(len(df.columns))
width = 0.13

for i, (method, values) in enumerate(df.iterrows()):
    ax.bar(x + i*width, values, width, label=method, color=colors[i])

# Customize the plot
ax.set_xlabel('')
ax.set_ylabel('runtime (seconds)', rotation=90, fontsize=18)
ax.yaxis.set_label_coords(-0.12, 0.425) 
ax.set_xticks(x + width*0.5)
ax.set_xticklabels(df.columns, fontsize=18)

ax.set_xlim(-0.5, len(df.columns) - 0.3)

# Set y-tick font size
ax.tick_params(axis='y', labelsize=18)

# Add grid
ax.grid(True, axis='y', linestyle='-', alpha=0.3)
ax.set_axisbelow(True)

# Clean up the plot
ax.spines['top'].set_visible(False)
ax.spines['right'].set_visible(False)

# Save the main plot without legend
plt.tight_layout()
plt.savefig('figures/cxl_desired_vs_possible.pdf', bbox_inches='tight')

# Create separate legend
legend_fig, legend_ax = plt.subplots(figsize=(8, 1))
legend_ax.axis('off')

# Create legend elements
improved_labels = [method for i, (method, values) in enumerate(df.iterrows())]
legend_elements = [plt.Rectangle((0,0),1,1, fc=colors[i], label=improved_labels[i])
                   for i in range(len(improved_labels))]

# Create legend (single row since we only have 2 items)
legend_ax.legend(handles=legend_elements,
                loc='center',
                ncol=2,
                frameon=False,
                fontsize=28,
                handletextpad=0.5)

# Save the legend PDF
legend_fig.savefig('figures/cxl_desired_vs_possible_legend.pdf', bbox_inches='tight', pad_inches=0.1)
print("Saved figures/cxl_desired_vs_possible.pdf")
print("Saved figures/cxl_desired_vs_possible_legend.pdf")

plt.show()
