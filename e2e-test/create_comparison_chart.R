#!/usr/bin/env Rscript
suppressPackageStartupMessages({
  library(ggplot2)
  library(dplyr)
  library(tidyr)
})

# Read comparison data
data <- read.csv("comparison_data.csv")

# Create long format for plotting
data_long <- data %>%
  pivot_longer(cols = c(azure_units, oci_units), 
               names_to = "system", 
               values_to = "units") %>%
  mutate(system = ifelse(system == "azure_units", "Azure pcd-server-01", "OCI perftest-vm-02"))

# Plot 1: CPU Units Comparison
p1 <- ggplot(data_long, aes(x = busy_pct, y = units, color = system, shape = system)) +
  geom_line(linewidth = 1.2) +
  geom_point(size = 3) +
  scale_color_manual(values = c("Azure pcd-server-01" = "#0078D4", "OCI perftest-vm-02" = "#F80000")) +
  labs(
    title = "CPU Calibration Comparison: Azure vs OCI",
    subtitle = "Work units required per CPU busy percentage",
    x = "CPU Busy %",
    y = "Work Units",
    color = "System",
    shape = "System"
  ) +
  theme_minimal() +
  theme(
    plot.title = element_text(size = 16, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "bottom",
    panel.grid.minor = element_blank()
  ) +
  scale_x_continuous(breaks = seq(10, 100, 10))

ggsave("visualizations/cpu_calibration_comparison.png", p1, width = 10, height = 6, dpi = 150)
cat("Created: visualizations/cpu_calibration_comparison.png\n")

# Plot 2: Performance Ratio
p2 <- ggplot(data, aes(x = busy_pct, y = ratio)) +
  geom_hline(yintercept = mean(data$ratio), linetype = "dashed", color = "gray50", linewidth = 0.8) +
  geom_line(color = "#6B5B95", linewidth = 1.2) +
  geom_point(color = "#6B5B95", size = 3) +
  annotate("text", x = 90, y = mean(data$ratio) + 0.02, 
           label = paste0("Avg: ", round(mean(data$ratio), 2), "x"), 
           color = "gray40", hjust = 0) +
  labs(
    title = "Performance Ratio: Azure / OCI",
    subtitle = "Higher ratio means Azure requires more work units for same CPU utilization",
    x = "CPU Busy %",
    y = "Ratio (Azure/OCI)"
  ) +
  theme_minimal() +
  theme(
    plot.title = element_text(size = 16, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    panel.grid.minor = element_blank()
  ) +
  scale_x_continuous(breaks = seq(10, 100, 10)) +
  scale_y_continuous(limits = c(1.4, 1.6), breaks = seq(1.4, 1.6, 0.05))

ggsave("visualizations/performance_ratio.png", p2, width = 10, height = 6, dpi = 150)
cat("Created: visualizations/performance_ratio.png\n")

# Plot 3: Bar chart comparison at key utilization levels
key_levels <- data %>% filter(busy_pct %in% c(25, 50, 75, 100))
if (nrow(key_levels) < 4) {
  key_levels <- data %>% filter(busy_pct %in% c(30, 50, 70, 100))
}

key_long <- key_levels %>%
  pivot_longer(cols = c(azure_units, oci_units), 
               names_to = "system", 
               values_to = "units") %>%
  mutate(system = ifelse(system == "azure_units", "Azure", "OCI"),
         busy_label = paste0(busy_pct, "%"))

p3 <- ggplot(key_long, aes(x = busy_label, y = units, fill = system)) +
  geom_bar(stat = "identity", position = "dodge", width = 0.7) +
  scale_fill_manual(values = c("Azure" = "#0078D4", "OCI" = "#F80000")) +
  labs(
    title = "Work Units by CPU Utilization Level",
    subtitle = "Side-by-side comparison at key utilization levels",
    x = "CPU Utilization",
    y = "Work Units",
    fill = "System"
  ) +
  theme_minimal() +
  theme(
    plot.title = element_text(size = 16, face = "bold"),
    plot.subtitle = element_text(size = 12, color = "gray40"),
    legend.position = "bottom",
    panel.grid.minor = element_blank()
  ) +
  geom_text(aes(label = units), position = position_dodge(width = 0.7), 
            vjust = -0.5, size = 3.5)

ggsave("visualizations/utilization_comparison_bar.png", p3, width = 10, height = 6, dpi = 150)
cat("Created: visualizations/utilization_comparison_bar.png\n")

cat("\nAll comparison charts generated successfully!\n")
