package com.guyu2233.ibukirpg.app.data

import androidx.compose.runtime.Immutable
import kotlinx.serialization.Serializable

// 0.2.0 开放世界 DTO（与 Go 端 internal/api/dto/world.go 对应）。

@Immutable @Serializable
data class WorldLogV1(
    val changes: List<WorldChangeV1> = emptyList(),
    val reveals: List<KnowledgeChipV1> = emptyList(),
    val rejected: Int = 0,
    /** parse / refused / validator_all_rejected；空 = 成功。 */
    val failed: String? = null,
    val impact: Int = 0,
)

@Immutable @Serializable
data class WorldChangeV1(
    val id: String = "",
    val op: String = "",
    val target: String = "",
    val targetName: String = "",
    val path: String? = null,
    val summary: String = "",
    val reason: String? = null,
    val before: String? = null,
    val after: String? = null,
    val source: String = "",
    val sourceLabel: String = "",
    val turn: Int = 0,
    val time: String? = null,
    val impact: Int = 0,
    val types: List<String> = emptyList(),
    val revertedBy: String? = null,
    val reverts: String? = null,
    val mechanical: Boolean = false,
    val hidden: Boolean = false,
    val canRevert: Boolean = false,
)

@Immutable @Serializable
data class KnowledgeChipV1(
    val entity: String = "",
    val name: String = "",
    val field: String = "",
    val fieldLabel: String = "",
    /** known / rumored */
    val level: String = "known",
    val channel: String? = null,
    /** field / relation / rumor */
    val kind: String? = null,
)

@Immutable @Serializable
data class DecisionV1(
    val id: String = "",
    val type: String = "",
    val typeLabel: String = "",
    val sensitivity: String = "",
    val title: String = "",
    val summary: String = "",
    val score: Int = 0,
    val lines: List<String> = emptyList(),
    val more: Int = 0,
    val notify: Boolean = false,
    val rollbackTurn: Int = 0,
    val rollbackText: String? = null,
    val checkpoint: String? = null,
)

@Immutable @Serializable
data class UsageV1(
    val turn: Int = 0,
    val calls: Int = 0,
    val promptTokens: Int = 0,
    val completionTokens: Int = 0,
    val failed: Int = 0,
    val tasks: List<UsageTaskV1> = emptyList(),
)

@Immutable @Serializable
data class UsageTaskV1(
    val task: String = "",
    val name: String = "",
    val model: String = "",
    val calls: Int = 0,
    val promptTokens: Int = 0,
    val completionTokens: Int = 0,
    val latencyMs: Long = 0,
    val failed: Int = 0,
)

@Immutable @Serializable
data class AdjudicationV1(
    val plausibility: String = "",
    val plausibilityLabel: String = "",
    val parse: String = "",
    val mods: List<String> = emptyList(),
    val modTotal: Int = 0,
    val dice: String? = null,
    val rolls: List<Int> = emptyList(),
    val total: Int = 0,
    val dc: Int = 0,
    val degree: String? = null,
    val degreeLabel: String? = null,
    val margin: Int = 0,
    val result: String? = null,
    val downgrade: String? = null,
    val band: String? = null,
)

@Immutable @Serializable
data class BranchV1(
    val id: String = "",
    val name: String = "",
    val parent: String? = null,
    val forkTurn: Int = 0,
    val headTurn: Int = 0,
    val status: String = "active",
    val current: Boolean = false,
)

@Immutable @Serializable
data class TimelineTurnV1(
    val turn: Int = 0,
    val time: String? = null,
    val summary: String = "",
    val current: Boolean = false,
    val fork: Boolean = false,
)

@Immutable @Serializable
data class CheckpointV1(
    val id: String = "",
    val branch: String = "",
    val turn: Int = 0,
    val name: String = "",
    /** auto / manual / daily */
    val kind: String = "",
    val reason: String? = null,
)

@Immutable @Serializable
data class TimelineV1(
    val branches: List<BranchV1> = emptyList(),
    val turns: List<TimelineTurnV1> = emptyList(),
    val checkpoints: List<CheckpointV1> = emptyList(),
    val pendingTurn: Int = 0,
    val branch: String = "",
)

@Immutable @Serializable
data class WorldEventChipV1(
    val id: String = "",
    val title: String = "",
    val status: String = "",
    val statusLabel: String = "",
    val location: String? = null,
    val countdown: String? = null,
    val startsInMin: Long = 0,
    val rumored: Boolean = false,
    val summary: String? = null,
    val outcome: String? = null,
    val time: String? = null,
    val pivotal: Boolean = false,
)

@Immutable @Serializable
data class EntityFieldV1(
    val key: String = "",
    val label: String = "",
    val value: String? = null,
    /** known / rumored / unknown */
    val level: String = "unknown",
    val changed: Boolean = false,
)

@Immutable @Serializable
data class EntityViewV1(
    val id: String = "",
    val kind: String = "",
    val name: String = "",
    val icon: String? = null,
    val fields: List<EntityFieldV1> = emptyList(),
    val secrets: List<String> = emptyList(),
    val retired: String? = null,
    val here: Boolean = false,
    val progress: String? = null,
)

@Immutable @Serializable
data class WorldPanelV1(
    val tab: String = "",
    val entities: List<EntityViewV1> = emptyList(),
    val events: List<WorldEventChipV1> = emptyList(),
    val changes: List<WorldChangeV1> = emptyList(),
    val relations: RelationsV1? = null,
    val codex: CodexV1? = null,
    val inventory: InventoryV1? = null,
)

// ---------- 设置：多服务商 + 生成设置 + 提示灵敏度 ----------

@Immutable @Serializable
data class ProviderV1(
    val id: String = "",
    /** deepseek / qwen / custom / llamacpp */
    val kind: String = "",
    val label: String = "",
    val baseUrl: String = "",
    val model: String = "",
    val models: List<String> = emptyList(),
    /** 只在发送给引擎时填写；持久化时密钥单独存放在加密存储里。 */
    val apiKey: String = "",
    val noTools: Boolean = false,
    val sizeB: Double = 0.0,
)

@Immutable @Serializable
data class RouteV1(val provider: String = "", val model: String = "")

@Immutable @Serializable
data class GenSettingsV1(
    /** unified / per_task */
    val mode: String = "unified",
    val unified: RouteV1 = RouteV1(),
    val tasks: Map<String, RouteV1> = emptyMap(),
    val auditEveryTurns: Int = 0,
    val showTokenUsage: Boolean = true,
    val retryOnRefusal: Boolean = false,
)

@Immutable @Serializable
data class SensitivityV1(
    /** off / low / mid / high */
    val level: String = "mid",
    /** modal / notify */
    val mode: String = "modal",
)

@Immutable @Serializable
data class PromptSettingsV1(
    val loreDeviation: SensitivityV1 = SensitivityV1(),
    val majorDeath: SensitivityV1 = SensitivityV1(),
    val storyImpact: SensitivityV1 = SensitivityV1(),
    val autoCheckpoint: Boolean = true,
    val keepAuto: Int = 20,
    val dailyCheckpoint: Boolean = true,
)

@Serializable
data class AIConfigV2(val providers: List<ProviderV1>, val settings: GenSettingsV1, val prompts: PromptSettingsV1? = null)

@Immutable @Serializable
data class TaskInfoV1(
    val id: String = "",
    val name: String = "",
    val output: String = "",
    val suggest: String = "",
    /** good / caution / not_recommended */
    val localFit: String = "",
    val localWhy: String = "",
)

@Immutable @Serializable
data class TasksV1(val tasks: List<TaskInfoV1> = emptyList(), val status: AIStatusV1 = AIStatusV1())

// ---------- 存档导出 / 导入 ----------

@Immutable @Serializable
data class SaveCheckV1(
    val ok: Boolean = false,
    val problems: List<String> = emptyList(),
    val warnings: List<String> = emptyList(),
)

@Immutable @Serializable
data class ExportResultV1(val path: String = "", val name: String = "", val bytes: Long = 0)

@Immutable @Serializable
data class ImportSaveResultV1(val slotId: String = "", val check: SaveCheckV1 = SaveCheckV1())

/** 一组修改的预览（玩家明确要求的修改先预览、后确认）。 */
@Immutable @Serializable
data class ChangePreviewV1(
    val previewToken: String = "",
    val changes: List<WorldChangeV1> = emptyList(),
    val hiddenCount: Int = 0,
    val impact: Int = 0,
    val types: List<String> = emptyList(),
    val rejected: List<RejectV1> = emptyList(),
    val note: String = "",
)

@Immutable @Serializable
data class RejectV1(val target: String = "", val path: String = "", val op: String = "", val reason: String = "")

/** 一致性审查结果（叙事流 kind=audit）。 */
@Immutable @Serializable
data class AuditV1(
    val id: String = "",
    val findings: List<AuditFindingV1> = emptyList(),
    val fixed: WorldLogV1? = null,
    val suggestions: List<AuditSuggestionV1> = emptyList(),
)

@Immutable @Serializable
data class AuditFindingV1(val kind: String = "", val turn: Int = 0, val text: String = "", val note: String = "")

@Immutable @Serializable
data class AuditSuggestionV1(val id: String = "", val summary: String = "", val reason: String = "", val status: String = "pending")
