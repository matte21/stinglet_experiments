import matplotlib as mpl
import matplotlib.pyplot as plt
import pandas as pd
import numpy as np

# This script produces FIGURE 2 in the HCDS paper.

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

# Read v2 CSV
df = pd.read_csv('data/kubelet_mem_underutil.csv', skiprows=2, index_col=0)

# Transpose so time points are rows and metrics are columns
df = df.T

# Set up the plot
fig, ax1 = plt.subplots(figsize=(7, 3))

# Define colors
colors = ['#4472C4', '#E15759'] # Green for node 1, Yellow for node 2

# X-axis positions
x = np.arange(len(df))
width = 0.2  # Width of bars

# Plot data on the single left axis
ax1.bar(x - width/2, df['free GiB on node 1'], width, color=colors[0], 
        label='free GiB on node 1')
ax1.bar(x + width/2, df['free GiB on node 2'], width, color=colors[1], 
        label='free GiB on node 2')

# Add annotations with arrows for zero values
for col in df.columns:
    for i, val in enumerate(df[col]):
        if val == 0:
            if 'node 1' in col:
                bar_x = x[i] - width/2
                color = colors[0]
            else:
                bar_x = x[i] + width/2
                color = colors[1]
            
            ax1.annotate('0', xy=(bar_x, val), xytext=(bar_x, val + 1.2),
                         fontsize=12, ha='center', color=color,
                         arrowprops=dict(arrowstyle='->', color=color, lw=1.5))

# Customize y-axis (GiB)
ax1.set_ylabel('GiB', rotation=90, fontsize=14)
ax1.set_yticks([5, 10])
ax1.set_ylim(0, 12)
ax1.tick_params(axis='y', labelsize=12)

# Customize x-axis
# Increased labelpad from 35 to 55 to add more space
ax1.set_xlabel('time →', fontsize=14, labelpad=42)
ax1.set_xticks(x)
ax1.set_xticklabels([]) # Hide default labels

# Custom Label Rendering
trans = ax1.get_xaxis_transform() # x in data coords, y in axes coords

for i, label in enumerate(df.index):
    if "App 1 arrives" in label:
        # Line 1: "App 1 arrives:"
        ax1.text(x[i], -0.08, "App 1 arrives:", 
                 ha='center', va='top', transform=trans, 
                 fontsize=12, color='black', family='serif')
        
        # Line 2 Text: "admitted to host"
        ax1.text(x[i], -0.18, "admitted to host", 
                 ha='center', va='top', transform=trans, 
                 fontsize=12, color='black', family='serif')
        
        # Line 2 Symbol: Checkmark (Green, Sans-serif)
        ax1.text(x[i] + 0.32, -0.18, "\u2713", 
                 ha='left', va='top', transform=trans, 
                 fontsize=18, color='#38761d', family='DejaVu Sans', fontweight='bold')
                 
    elif "App 2 arrives" in label:
        # Line 1: "App 2 arrives:"
        ax1.text(x[i], -0.08, "App 2 arrives:", 
                 ha='center', va='top', transform=trans, 
                 fontsize=12, color='black', family='serif')
        
        # Line 2 Text: "rejected from host"
        ax1.text(x[i], -0.18, "rejected from host", 
                 ha='center', va='top', transform=trans, 
                 fontsize=12, color='black', family='serif')
        
        # Line 2 Symbol: Cross (Red, Sans-serif)
        ax1.text(x[i] + 0.37, -0.18, "\u2717", 
                 ha='left', va='top', transform=trans, 
                 fontsize=18, color='#cc0000', family='DejaVu Sans', fontweight='bold')
                 
    else:
        # Standard label
        ax1.text(x[i], -0.08, label, 
                 ha='center', va='top', transform=trans, 
                 fontsize=12, color='black', family='serif')

# Add grid
ax1.grid(True, axis='y', linestyle='-', alpha=0.3)
ax1.set_axisbelow(True)

# Clean up spines
ax1.spines['top'].set_visible(False)
ax1.spines['right'].set_visible(False)

# Save the plot
plt.tight_layout()
plt.savefig('figures/kubelet_mem_underutil.pdf', bbox_inches='tight')
print("Plot saved as figures/kubelet_mem_underutil.pdf")

# Create legend
fig_legend, ax_legend = plt.subplots(figsize=(8, 1))
ax_legend.axis('off')

handles, labels = ax1.get_legend_handles_labels()

legend = ax_legend.legend(handles, labels, loc='center', 
                          ncol=2, frameon=False, fontsize=12)

plt.savefig('figures/kubelet_mem_underutil_legend.pdf', bbox_inches='tight')
print("Legend saved as figures/kubelet_mem_underutil_legend.pdf")
