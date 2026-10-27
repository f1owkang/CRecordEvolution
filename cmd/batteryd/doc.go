// Package main 实现 batteryd 守护进程：读取电池 sysfs 节点、结算充放电会话、
// 估算电池容量，并把结果输出为 module.prop 描述（daemon/once）与 JSON
// （json/history 子命令）。
//
// 数据流：Tick 每拍读 sysfs（电压/电流/温度/显示电量/charge_counter），充电期
// 累计会话并在状态翻转时结算；Estimator 把会话隐含容量融合进 EMA 基线；Store
// 负责 SQLite 十表的读写与 90 天清理；jsonout/history 按「缺失即省略」的纪律
// 把结果映射为 WebUI 契约。
//
// 容量多口径（代码与文档里复用的术语）：
//   - ema：会话隐含容量经 EMA 融合后的「实测」基线（estimator_stable.go /
//     estimator_ml.go）；
//   - mid：中段分窗校准隐含容量（midwin.go，样本电量计差分优先）；
//   - dis：放电差分隐含容量（discharge.go，不经 current_now）；
//   - full：内核 charge_full 自学习值（main.go 的 basics 与 quality.go 互检）。
//
// 单位约定：*UV=µV（电压）、*UA=µA（电流）、*UAh=µAh（容量）、*UAs=µA·s
// （电量积分）。换算只在展示层做，采样、积分、kv 与 JSON 出口都传微单位原值。
//
// 文件职责导览：
//   - main.go：子命令入口、app 装配、本地时区、描述/once/json 出口；
//   - pipeline.go：采样状态机、充电会话累计与结算、内阻与静置点；
//   - discharge.go：放电会话差分累计与校准；
//   - store.go：SQLite 建表、幂等迁移、各表读写与 90 天清理；
//   - sysfs.go：sysfs 节点查找、整数读取与电流/温度单位归一；
//   - estimator*.go：stable / ml 两条容量估计通道；
//   - midwin.go、ccct.go、ccseg.go、ica.go：中段校准与 CCCT/ICA 特征；
//   - quality.go：ema/mid/dis/full 四路容量口径互检；
//   - trend.go：容量趋势三态判定（Mann-Kendall）；
//   - describe.go、i18n.go：module.prop 描述与双语文案；
//   - jsonout.go、history.go：WebUI 取数协议出口。
package main
