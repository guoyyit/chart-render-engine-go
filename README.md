# chart-render-engine-go

把结构化数据渲染成可直接嵌入页面的 SVG 矢量文本的服务端 Go 引擎。
同一份输入在任何一次运行、任何并发任务中都产出**逐字节一致**的结果，
便于做快照比对与回归排查。

- 无外部依赖，纯标准库实现
- 输出为字符串，不写文件、不依赖外部字体与平台环境
- 渲染过程可取消（`context.Context`），取消后无半成品输出、无资源泄漏

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    chart "github.com/guoyyit/chart-render-engine-go"
)

func main() {
    spec := chart.Spec{
        Width:  800,
        Height: 500,
        Title:  "示例图表",
        Series: []chart.Series{
            {Name: "销售额", Type: chart.SeriesLine, Y: []float64{12, 18, 15, 25, 22}},
        },
    }
    result, err := chart.Render(context.Background(), spec)
    if err != nil {
        panic(err)
    }
    fmt.Println(result.SVG)         // 矢量文本
    for _, d := range result.Diagnostics {
        fmt.Println(d.Code, d.Message) // 数据/配置诊断
    }
}
```

## 图表描述格式

### Spec（图表描述）

| 字段 | 说明 |
| --- | --- |
| `Width`, `Height` | 画布尺寸（像素），必须 > 0 |
| `Title` | 标题，居中显示 |
| `Background` | 背景色（SVG 颜色串）；空串为白色，`"transparent"` 不画背景 |
| `Series` | 系列列表，**按切片顺序绘制**，后画的系列叠加在先画的之上 |
| `Axes` | 坐标轴列表；为空时自动创建 `x`（底部、线性）与 `y`（左侧、线性） |
| `Legend` | 图例配置，`Hidden: true` 可关闭图例 |

### Series（数据系列）

| 字段 | 说明 |
| --- | --- |
| `Name` | 图例名称；为空则不显示图例项 |
| `Type` | `SeriesLine`（折线）、`SeriesArea`（面积）、`SeriesBar`（柱状）、`SeriesStackedBar`（堆叠柱状）、`SeriesScatter`（散点） |
| `X` | x 坐标；为 `nil` 时使用点序号 0,1,2,… |
| `Y` | y 坐标；`NaN` 表示缺失值 |
| `Color` | SVG 颜色串；为空时按系列顺序从内置调色板取色 |
| `Opacity` | 不透明度 0..1；0 表示完全不透明（默认） |
| `XAxis` / `YAxis` | 引用的坐标轴 ID，默认 `"x"` / `"y"` |
| `StackGroup` | 堆叠组名，默认 `"default"`；同组堆叠柱按系列顺序累加 |
| `BarWidth` | 柱宽（x 轴数据单位）；0 为自动 |

### AxisSpec（坐标轴）

| 字段 | 说明 |
| --- | --- |
| `ID` | 系列通过 `XAxis`/`YAxis` 引用此 ID |
| `Kind` | `AxisLinear`（线性）、`AxisLog`（对数）、`AxisTime`（时间） |
| `Position` | `AxisBottom`、`AxisTop`、`AxisLeft`、`AxisRight` |
| `Min` / `Max` | 显式指定域上下界（指针）；`nil` 表示自动推导 |
| `Base` | 对数轴的底；0 表示 10 |
| `Title` | 轴标题 |

## 坐标语义

### 线性轴（AxisLinear）

- 域由数据最小值/最大值经 **Nice Domain** 算法扩展为语义友好边界
  （步长取自 1、2、5 × 10^k 序列）。
- 刻度步长同样是 1/2/5 × 10^k，覆盖整个域，标签不会出现长浮点尾差
  （如 `0.6000000000000001`）。
- **数据全部相同**时，域自动向两侧扩展（如 42 → [38, 46]）；
  **数据为空**时使用默认域 [0, 1]，结果稳定。

### 对数轴（AxisLog）

- 默认以 10 为底，主刻度为底的整数次幂（1, 10, 100, …），
  窄域时自动补充 2、5 刻度。
- 非正值（≤ 0）不能取对数：这些点被跳过并输出 `LOG_NON_POSITIVE` 诊断，
  不会被当作 0。
- 显式 bounds 必须为正，否则输出 `INVALID_AXIS_BOUNDS` 并回退自动推导。

### 时间轴（AxisTime）

- 域值为 **unix 秒**（`time.Unix()`），全部按 UTC 计算，不受渲染机时区影响。
- 刻度按日历/时钟边界对齐：秒、分、时、日、周（周一）、月、年，
  标签格式随跨度自动选择（`15:04:05` / `15:04` / `2006-01-02` /
  `2006-01` / `2006`）。

### 共享轴与独立轴

- **共享**：多个系列引用同一轴 ID → 共用同一套刻度与映射，
  系列间的数值可以直接比较（如两个系列共用左 y 轴）。
- **独立**：不同系列引用不同轴 ID（如左 `y` 与右 `y2`）→ 各自独立刻度，
  但共享同一个绘图区，对齐关系可解释：所有轴的像素范围都是绘图区矩形，
  零值线在线性轴上会绘制为参考线。

## 降采样

当数据点规模超过像素承载能力时自动降采样，输出体积随点数增长保持可控
（约每像素 2 个点 / 柱）：

- **折线 / 面积 / 散点**：按段（缺失值分段）分桶，每桶保留
  **最小值点、最大值点**，并保留**段首、段尾**点；并列极值取更早的点，
  结果完全确定。
- **柱状 / 堆叠柱状**：按 x 范围分桶，桶内求和（堆叠柱因此保持正确），
  桶宽随绘图区自适应。
- 降采样只对**有效点**生效：缺失值（NaN）不参与极值与求和计算。

## 缺失值与非法值

输入问题被稳妥处理，不会崩溃，也不会把缺失值当作 0：

| 诊断码 | 级别 | 含义 |
| --- | --- | --- |
| `MISSING_VALUES` | info | y 为 NaN（缺失）；不绘制、不参与坐标范围与统计 |
| `INVALID_VALUES` | warning | 存在 Inf 或 NaN 坐标；点被跳过 |
| `LOG_NON_POSITIVE` | warning | 对数轴上的非正值；点被跳过 |
| `EMPTY_SERIES` | warning | 系列无数据，已跳过 |
| `LENGTH_MISMATCH` | error | x 与 y 长度不一致，系列已跳过 |
| `AXIS_NOT_FOUND` | error | 系列引用了不存在的轴 ID，已跳过 |
| `INVALID_AXIS_BOUNDS` | error | 显式轴界非法，已回退自动推导 |
| `EMPTY_AXIS_DOMAIN` | warning | 轴上无有效数据，使用默认域 |
| `DOWNSAMPLED` | info | 折线/面积/散点已降采样（含点数变化） |
| `AGGREGATED` | info | 柱状数据已聚合（含桶数变化） |
| `INVALID_OPACITY` | warning | 不透明度越界，已钳制 |
| `INVALID_DIMENSIONS` | error | 画布尺寸非法 |
| `NO_SERIES` | warning | 没有任何系列 |

## 布局

- 标题居中、图例在标题下方自动换行，绘图区位于标题/图例下方，
  互不重叠。
- 坐标轴边距根据刻度标签宽度与轴标题长度自适应。
- x 轴标签过密时按**确定规则抽稀**：按标签总宽度与绘图区宽度的比值
  计算步长，保留首、尾刻度；y 轴同理。
- 画布尺寸变化时布局重新计算，标签宽度估算不依赖平台字体。

## 确定性与并发

- 渲染为纯函数：无全局可变状态、无 map 迭代顺序依赖、无随机数、
  无时间相关输出；同一份输入重复渲染、并发渲染均逐字节一致。
- 颜色从固定调色板按系列序选取，也可通过 `Color` 显式指定。
- `Render` 接收 `context.Context`：在阶段边界检查取消信号；
  取消时返回 `ctx.Err()` 且结果为 `nil`，不产生半成品输出，
  不启动 goroutine、不写文件，因此无资源泄漏。

## API

```go
func Render(ctx context.Context, spec Spec) (*Result, error)

type Result struct {
    SVG         string        // 矢量文本
    Diagnostics []Diagnostic  // 数据/配置诊断，按处理顺序排列
}
```
