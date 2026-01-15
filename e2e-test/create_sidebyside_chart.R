#!/usr/bin/env Rscript
suppressPackageStartupMessages({
  library(ggplot2)
  library(dplyr)
  library(tidyr)
  library(gridExtra)
})

# Read comparison data
data <- read.csv("comparison_data.csv")

# Create long format for plotting
data_long <- data %>%
  pivot_longer(cols = c(azure_units, oci_units), 
               names_to = "system", 
               values_to = "units") %>%
  mutate(system = ifelse(system == "azure_units", "Azure pcd-server-01", "OCI perftest-vm-02"))

# Color scheme
azure_color <- "#0078D4"
oci_color <- "#F80000"

# ============================================================================
# COMPREHENSIVE SIDE-BY-SIDE DASHBOARD
# ============================================================================

# Plot 1: Side-by-side line comparison
p1 <- ggplot(data_long, aes(x = busy_pct, y = units, color = system)) +
  geom_line(linewidth = 1.5) +
  geom_point(size = 4) +
  scale_color_manual(values = c("Azure pcd-server-01" = azure_color, "OCI perftest-vm-02" = oci_color)) +
  labs(
    title = "CPU Performance Comparison",
    subtitle = "Work units required at each CPU utilization level",
    x = "CPU Busy %",
    y = "Work Units",
    color = NULL
  ) +
  theme_minimal(base_size = 14) +
  theme(
    plot.title = element_text(size = 18, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "bottom",
    panel.grid.minor = element_blank(),
    panel.border = element_rect(color = "gray80", fill = NA)
  ) +
  scale_x_continuous(breaks = seq(10, 100, 10))

# Plot 2: Faceted comparison (side by side panels)
p2 <- ggplot(data_long, aes(x = busy_pct, y = units, fill = system)) +
  geom_area(alpha = 0.7) +
  geom_line(color = "white", linewidth = 0.5) +
  facet_wrap(~system, ncol = 2) +
  scale_fill_manual(values = c("Azure pcd-server-01" = azure_color, "OCI perftest-vm-02" = oci_color)) +
  labs(
    title = "Performance Profile by System",
    subtitle = "Area chart showing compute capacity",
    x = "CPU Busy %",
    y = "Work Units"
  ) +
  theme_minimal(base_size = 14) +
  theme(
    plot.title = element_text(size = 18, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "none",
    panel.grid.minor = element_blank(),
    strip.text = element_text(size = 14, face = "bold"),
    strip.background = element_rect(fill = "gray95", color = NA)
  ) +
  scale_x_continuous(breaks = seq(20, 100, 20))

# Plot 3: Bar chart comparison at all levels
p3 <- ggplot(data_long, aes(x = factor(busy_pct), y = units, fill = system)) +
  geom_bar(stat = "identity", position = position_dodge(width = 0.8), width = 0.7) +
  scale_fill_manual(values = c("Azure pcd-server-01" = azure_color, "OCI perftest-vm-02" = oci_color)) +
  labs(
    title = "Work Units at Each Utilization Level",
    subtitle = "Direct comparison across all CPU busy percentages",
    x = "CPU Busy %",
    y = "Work Units",
    fill = NULL
  ) +
  theme_minimal(base_size = 14) +
  theme(
    plot.title = element_text(size = 18, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "bottom",
    panel.grid.minor = element_blank(),
    panel.border = element_rect(color = "gray80", fill = NA)
  ) +
  geom_text(aes(label = units), position = position_dodge(width = 0.8), 
            vjust = -0.5, size = 3)

# Plot 4: Difference/Gap chart
data_diff <- data %>%
  mutate(
    difference = azure_units - oci_units,
    pct_diff = (azure_units - oci_units) / oci_units * 100
  )

p4 <- ggplot(data_diff, aes(x = busy_pct, y = difference)) +
  geom_bar(stat = "identity", fill = "#6B5B95", width = 6) +
  geom_text(aes(label = paste0("+", difference)), vjust = -0.5, size = 4) +
  labs(
    title = "Performance Gap (Azure - OCI)",
    subtitle = "Additional work units Azure can handle vs OCI",
    x = "CPU Busy %",
    y = "Work Unit Difference"
  ) +
  theme_minimal(base_size = 14) +
  theme(
    plot.title = element_text(size = 18, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    panel.grid.minor = element_blank(),
    panel.border = element_rect(color = "gray80", fill = NA)
  ) +
  scale_x_continuous(breaks = seq(10, 100, 10))

# Save individual plots
ggsave("visualizations/sidebyside_line.png", p1, width = 12, height = 7, dpi = 150)
ggsave("visualizations/sidebyside_facet.png", p2, width = 14, height = 6, dpi = 150)
ggsave("visualizations/sidebyside_bars.png", p3, width = 14, height = 7, dpi = 150)
ggsave("visualizations/performance_gap.png", p4, width = 12, height = 6, dpi = 150)

cat("Created: visualizations/sidebyside_line.png\n")
cat("Created: visualizations/sidebyside_facet.png\n")
cat("Created: visualizations/sidebyside_bars.png\n")
cat("Created: visualizations/performance_gap.png\n")

# ============================================================================
# COMBINED DASHBOARD
# ============================================================================

# Create a combined dashboard with all plots
png("visualizations/performance_dashboard.png", width = 1800, height = 1400, res = 120)
grid.arrange(
  p1, p2, p3, p4,
  ncol = 2, nrow = 2,
  top = grid::textGrob("Azure pcd-server-01 vs OCI perftest-vm-02: Performance Dashboard", 
                       gp = grid::gpar(fontsize = 22, fontface = "bold"))
)
dev.off()
cat("Created: visualizations/performance_dashboard.png\n")

# ============================================================================
# SUMMARY STATS VISUALIZATION
# ============================================================================

# Create summary stats
summary_data <- data.frame(
  metric = c("Max Work Units", "Avg Work Units", "Linear Slope", "Efficiency Ratio"),
  Azure = c(max(data$azure_units), round(mean(data$azure_units), 1), 2.74, 1.51),
  OCI = c(max(data$oci_units), round(mean(data$oci_units), 1), 1.83, 1.00)
)

summary_long <- summary_data %>%
  pivot_longer(cols = c(Azure, OCI), names_to = "system", values_to = "value")

p5 <- ggplot(summary_long, aes(x = metric, y = value, fill = system)) +
  geom_bar(stat = "identity", position = position_dodge(width = 0.8), width = 0.7) +
  scale_fill_manual(values = c("Azure" = azure_color, "OCI" = oci_color)) +
  labs(
    title = "Performance Summary Metrics",
    subtitle = "Key performance indicators comparison",
    x = NULL,
    y = "Value",
    fill = "System"
  ) +
  theme_minimal(base_size = 14) +
  theme(
    plot.title = element_text(size = 18, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "bottom",
    panel.grid.minor = element_blank(),
    axis.text.x = element_text(angle = 15, hjust = 1)
  ) +
  geom_text(aes(label = round(value, 2)), position = position_dodge(width = 0.8), 
            vjust = -0.5, size = 4)

ggsave("visualizations/summary_metrics.png", p5, width = 12, height = 7, dpi = 150)
cat("Created: visualizations/summary_metrics.png\n")

cat("\n=== All side-by-side visualizations generated! ===\n")
