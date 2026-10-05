// Chart cards for the admin dashboard (pure React components; logic lives in *.logic.ts).

export { ChartCardShell, ChartFootnote } from "./ChartCardShell";

export { LineChartCard } from "./LineChartCard";
export type { LineChartCardProps } from "./LineChartCard";
export type { LineChartInput, LineSeriesInput, LineMarkInput, LineTone } from "./line-chart.logic";

export { BarChartCard } from "./BarChartCard";
export type { BarChartCardProps } from "./BarChartCard";
export type { BarChartInput, BarLabelMode, BarShowValues, BarHighlight } from "./bar-chart.logic";

export { HBarListCard } from "./HBarListCard";
export type { HBarListCardProps } from "./HBarListCard";
export type { HBarItem, HBarTone, HBarFormat, HBarSort } from "./hbar-list.logic";

export { HeatmapCard } from "./HeatmapCard";
export type { HeatmapCardProps } from "./HeatmapCard";
export type { HeatmapInput, HeatmapTone } from "./heatmap.logic";

export { FunnelCard } from "./FunnelCard";
export type { FunnelCardProps } from "./FunnelCard";
export type { FunnelStepInput } from "./funnel.logic";

export { QuadrantCard } from "./QuadrantCard";
export type { QuadrantCardProps } from "./QuadrantCard";
export type { QuadrantPointInput, QuadrantKey } from "./quadrant.logic";

export { TimelineCard } from "./TimelineCard";
export type { TimelineCardProps } from "./TimelineCard";
export type { TimelineIncidentInput, TimelineSeverity } from "./timeline.logic";

export { SplitBarCard } from "./SplitBarCard";
export type { SplitBarCardProps } from "./SplitBarCard";
export type { SplitSegmentInput, SplitGroupInput, SplitTone, SplitShowValue } from "./split-bar.logic";
