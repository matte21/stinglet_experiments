#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source ../shared/env.sh

# Download and unzip the trace: it's 173 MiB.
readonly trace_url="https://azurepublicdatasettraces.blob.core.windows.net/azurepublicdatasetv2/azurevmallocation_dataset2020/AzurePackingTraceV1.zip"
readonly trace_zip="trace.zip"
if [[ -f "$trace_db_file" || -f "$trace_zip" ]]; then
    echo "Not downloading trace since it already exists."
else
    echo "Downloading trace..."
    curl "$trace_url" -o "$trace_zip"
fi
if [[ ! -f "$trace_db_file" ]]; then
    unzip "$trace_zip"
    rm "$trace_zip"
fi

echo

# The trace is a bunch of tables in a sqlite DB. We're about to manipulate that DB, so we need the
# sqlite3 CLI tool installed (if it's not already). 
if ! sqlite3 --version > /dev/null 2>&1; then
    echo "installing sqlite3 since it's missing!"
    sudo apt-get update -y
    sudo apt-get install -y sqlite3
else
    echo "sqlite3 already installed."
fi

echo

# The Azure trace is from the pre-CXL days, and doesn't tell us which VMs can and can't use CXL
# memory. To empirically ground our evaluation, we run two versions of each trace: one where 37% of
# VMs can use CXL memory, and one where 20% of VMs can. The two numbers are from an empirical
# analysis in Microsoft data centers, in the Pond and GreenSKU papers, respectively. Note that in
# those papers they either put all the memory of a CXL VM in CXL memory (greenSKU), or use ML to
# decide the split between normal and CXL memory. For simplicity, we randomly choose between
# 5%, 10%, 50% and 100% of the VM memory request in CXL memory.
readonly percentage_of_cxl_vms_pond=37
readonly percentage_of_cxl_vms_greensku=20

# Create additional tables. The resulting DB size is 2.7 GiB. 
readonly machine_ids=(4 10 17 20 27 21 16 11 1 6 34 19)
for mid in "${machine_ids[@]}"; do
    if ! sqlite3 "$trace_db_file" ".tables" | grep -qw "events-machine-id-${mid}"; then

        echo "creating arrival and departures table for machine type $mid..." 

        sqlite3 "$trace_db_file" << EOF

BEGIN;

CREATE TABLE "events-machine-id-${mid}" (
    vmId    INTEGER NOT NULL,
    priority INTEGER NOT NULL,
    cores   REAL,
    memory  REAL,
    cxl_mem_percent_pond INTEGER,
    cxl_mem_percent_greensku INTEGER,
    time    REAL
);

INSERT INTO "events-machine-id-${mid}" (vmId, priority, cores, memory, cxl_mem_percent_pond, cxl_mem_percent_greensku, time)
SELECT
    vm.vmId,
    vm.priority,
    vt.core,
    vt.memory,
    CASE WHEN ABS(RANDOM()) % 100 < ${percentage_of_cxl_vms_pond}
        THEN CASE ABS(RANDOM()) % 4
            WHEN 0 THEN 5
            WHEN 1 THEN 10
            WHEN 2 THEN 50
            WHEN 3 THEN 100
        END
        ELSE 0
    END,
    CASE WHEN ABS(RANDOM()) % 100 < ${percentage_of_cxl_vms_greensku}
        THEN CASE ABS(RANDOM()) % 4
            WHEN 0 THEN 5
            WHEN 1 THEN 10
            WHEN 2 THEN 50
            WHEN 3 THEN 100
        END
        ELSE 0
    END,
    vm.starttime
FROM vm
JOIN vmType vt ON vm.vmTypeId = vt.vmTypeId AND vt.machineId = ${mid}
WHERE vm.starttime >= 0;

INSERT INTO "events-machine-id-${mid}" (vmId, priority, cores, memory, time)
SELECT vm.vmId, vm.priority, -vt.core, -vt.memory, vm.endtime
FROM vm
JOIN vmType vt ON vm.vmTypeId = vt.vmTypeId AND vt.machineId = ${mid}
WHERE vm.starttime >= 0 AND vm.endtime IS NOT NULL;

COMMIT;

EOF
    else
        echo "arrival and departures table for machine type $mid already exists" 
    fi
done

echo
echo "trace database has size $(du -sh "$trace_db_file" | cut -f1)"

# machine type = physical server
# vm type = pre-configured VM sizes.
# In the trace, there are 35 distinct machine types as per:
# sqlite3 packing_trace_zone_a_v1.sqlite "SELECT COUNT(DISTINCT machineId) FROM vmType"
# and 488 distinct vm types as per:
# sqlite3 packing_trace_zone_a_v1.sqlite "SELECT COUNT(DISTINCT vmTypeId) FROM vmType"  
# There are 4619 distinct <vm type, machine type> pairs, so clearly a given machine type does not
# support every vm type (otherwise there would be 488*35 = 17080 distinct pairs).

# Note that in the trace the resource requests of each VM type are normalized to the capacity of each
# machine type where the VM type can run. Since our simulation is from the perspective of a single
# host, we should choose the machine type of the host, to know how many resources each VM wants.
# This leads to a problem: what happens if a VM of a type unsupported on the chosen machine type
# arrives? Idea: 
# Idea 1: look whether there are machine types that support all VM types, and use only those machine
# types, so the problem disappears.
# Idea 2: just ignore that VM. That makes sense: in a real cluster that VM wouldn't be allowed to
# run on our host. As a side, we should pick a handful of machine types for thoroughness, not just
# 1.
# Running:
# sqlite3 packing_trace_zone_a_v1.sqlite "SELECT machineId, COUNT(*) as cnt FROM vmType GROUP BY machineId ORDER BY cnt DESC"
# reveals that there are no machines types that host every VM. So we'll go with option 2.

# Another issue: some VMs keep running beyond the end of the trace. So the host might soon get
# filled with just those VMs and no longer see any departure and new arrival. On further reflection,
# this isn't an issue at all, it's ok if it happens as it would happen in reality.

# A real issue: some VMs were running before the beginning of the trace. We don't know the order of
# arrival of those VMs.
# Idea 1: just select a random order of arrival for those VMs.
# Idea 2: ignore those VMs and just consider those that arrive after the start of the trace. This
# means that in the simulation our host is a new host added to the cluster after the trace starts.
# Let's go with idea 2, I feel it leads to a more realistic experiment.
# A related issue is that VMs can have the same arrival time in general. How do we know their actual
# order of arrival in that case? We don't, and we just create them in the same order in which they
# appear in the DB. This means that sometimes we might saturate the host with VMs that are all
# arriving at the same time.
# Actually, on more careful inspection of the data, negative start times denote pre-existing VMs
# and are ordered too => we know their order of arrival! Although no, the issue really does exist
# because some negative times are for example -7157.0, i.e. 20 years before the trace was collected
# so negative numbers are not accurate.

# Other issue: we don't know which VM is associated to which host. This contrasts with the alibaba
# trace, that tells us the host on which each container is running. So how do we know which VMs run on
# our host ? We simulate a cluster scheduler that always prefers our host. i.e. the scheduler keeps
# track of how much free memory and CPUs are left on our host, and if a new VM fits, it gets
# scheduled on it. Otherwise, we just ignore that VM, mimicking the fact that the scheduler would
# choose a different host for our VM.

# Other issue: we don't know how much CXL memory each VM can use. But our system assumes that VMs
# tells the scheduler how much CXL and normal memory they want. How do we solve it? We randomly select 37%
# of VMs and say that they can use CXL memory; the 37% number was a finding from Microsoft's Pond
# paper. FOr the VMs that can use CXL memory, we select at random with a uniform distribution between
# 5, 10, 50 and 100% of their memory request to be for CXL memory, so that we have some variation
# for the amount of requested CXL memory.

# Other issue: the trace mixes high and low priority VMs. If we translate it to the K8s world, the
# high priority VMs would be guaranteed pods, while low priority would be non-guaranteed Pods, so
# alignment and admission control would apply only to the high priority pods. How do we factor that
# in? Since our focus is managing guaranteed pods, ignore low-priority VMs. That's one approach,
# which has the con of underestimating the utilization achievable because some underutilized
# resources that a baseline is unable to allocated to guaranteed pods could still be utilized by
# low priority VMs. A different approach would be to treat even low prio VMs as hihg-prio. That
# way we try to run all workloads that would be running on the host. But because of NUMA allocation
# and alignment rules, we might end up rejecting low prio VMs that would not be rejected in reality. 
# NOte: sqlite3 packing_trace_zone_a_v1.sqlite "SELECT COUNT(*) FROM vm WHERE priority == 1"
# reveals that the number of low priority VMs (priority == 1) is roughly 10% of the total, so
# they're a minority.
# Third option: consider low priority ones as well, and map them to burstable Pods.

# On machine type selection: which machine types do we choose? There are 35, too many to consider
# them all: too many experiments, too much data to analyze, and the size of the DB storing the trace
# would grow too large (we want to add tables whose schema is more convenient than the native one
# for the code that actually plays the trace - but they take space. If we considered all machine
# types, the new DB would be 10 GiB). One thing that I'd like though, is this: each VM type should
# appear in at least one trace => for each VM type, we should pick at least one machine type that
# supports that VM type. This query:
# sqlite3 packing_trace_zone_a_v1.sqlite "
# SELECT DISTINCT machineId
# FROM vmType
# WHERE vmTypeId IN (
#     SELECT vmTypeId
#     FROM vmType
#     GROUP BY vmTypeId
#     HAVING COUNT(DISTINCT machineId) = 1
# )
# "
#
# gives all machine types that support VM types supported by only one machine type, so we must
# include them. The result is:
# 4
# 10
# 17
# 20
# 27
#
# Empirically I've found the following set of machine types IDs to cover all VM types:
# (4, 10, 17, 20, 27, 21, 16, 11, 1, 6, 34, 19), to verify, run the following query (output should
# be 488, the number of VM types):
# sqlite3 packing_trace_zone_a_v1.sqlite "
# SELECT COUNT(DISTINCT vmTypeId)
# FROM vmType
# WHERE machineId IN (4, 10, 17, 20, 27, 21, 16, 11, 1, 6, 34, 19)
# "
# 
# machine types 11 and 16 are redundant, but they are the top two machine types for number of VM
# types supported, and I wanted the top two (and bottom two) types, so I included both. 
#
# Other issue: we pick a percentage of VMs in the trace that use CXL (e.g. 37%). But the
# cluster-wide scheduler we use in the simulation greedily uses the host where stinglet/kubelet is
# running, and when it saturates it, it ignores the new VMs (to mimic them being scheduled to a
# different host). So in principle it can happen that the host gets saturated of normal memory
# without any VM using CXL running on it, and all VMs that request CXL are created after the
# saturation. So we never actually schedule VMs with CXL to stinglet/kubelet. I don't know whether
# that's an issue that occurs frequently enough to make the experiment useless - but the experiments
# automation should collect metrics to detect this scenario so that if it happens, we know and can
# change something.    
#
# Here's Claude interpreting an antagonistic reviewer:
# Here are the sharpest criticisms an adversarial reviewer would raise:
# ### 1. Single-host simulation from a cluster trace is fundamentally flawed
# The Azure trace captures cluster-wide scheduling decisions made by a sophisticated packing algorithm (Protean, per the paper). The VMs that ended up on any given host were placed there *because* the scheduler decided they fit well together. By simulating a greedy "always prefer our host" scheduler, you're creating a workload that was never meant to run on a single host — the resource demand mix, VM sizes, and arrival patterns are all artifacts of cluster-level decisions. Your results may say more about this mismatch than about your kubelet modifications.

# ### 2. The "always prefer our host" scheduler is unrealistically greedy
# A real scheduler would spread load across hosts. Your simulated scheduler will pack the host far more aggressively than reality, potentially saturating it quickly and making both your modified kubelet and the baseline look worse than they would in a real deployment. Any improvement your kubelet shows could simply be an artifact of this aggressive packing, not a genuine signal.

# ### 3. Ignoring pre-trace VMs introduces a cold-start bias
# Starting with an empty host means your simulation begins in a state that never occurs in production — a host in a live cluster always has some existing workload. This makes early packing decisions artificially easy (lots of free resources), which could flatter or disadvantage either system depending on how their admission logic behaves at low utilization. The reviewer would ask: why not use Idea 1 with a randomly shuffled order? At least that would warm up the host to a realistic occupancy level before measuring.

# ### 4. CXL memory assignment is not validated
# The 37% figure from Pond applies to a specific workload at Microsoft at a specific point in time. There's no reason to believe it generalizes to this Azure VM trace, which is from a different workload, a different time period, and a different context (VM allocation vs. memory tiering). The uniform distribution over {5, 10, 50, 100}% is entirely made up. A reviewer would ask: why these four values? Why uniform? This introduces a free parameter that could be tuned — consciously or not — to make your system look better.

# ### 5. Mapping VM priority to Pod QoS class is not justified
# The Azure trace priority (high/low) and Kubernetes QoS classes (Guaranteed/Burstable/BestEffort) are defined differently and have different semantics. Low-priority Azure VMs are evictable due to *capacity pressure at cluster level*, while Burstable Pods in Kubernetes have memory limits and CPU throttling semantics. Mapping one to the other without justification is an apples-to-oranges comparison that a reviewer would push back on hard.

# ### 6. No sensitivity analysis
# Your choices — which machine type(s) to use, the 37% CXL fraction, the CXL memory split distribution, ignoring low-priority VMs vs. including them — are all free parameters. A rigorous evaluation would show that results hold across a range of these parameters. Without that, a reviewer could argue your positive results only hold for your specific parameter choices.

# ### 7. The metric "packing effectiveness" is underspecified
# How exactly do you measure whether your kubelet packs better? Rejection rate? Average utilization over time? Peak utilization? These can tell very different stories. A system that accepts more VMs early but causes more rejections later (due to fragmentation) might look better or worse depending on which metric you choose and at what time window you measure.

# ### 8. The trace is from 2020 and reflects pre-CXL hardware
# The Azure trace predates widespread CXL deployment. The VM memory footprints and arrival patterns may not reflect workloads that would actually benefit from CXL memory tiering. A reviewer would question whether this is the right trace to evaluate a CXL-aware scheduler at all.