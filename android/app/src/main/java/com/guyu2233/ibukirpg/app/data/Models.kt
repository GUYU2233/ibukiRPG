package com.guyu2233.ibukirpg.app.data

import androidx.compose.runtime.Immutable
import kotlinx.serialization.Serializable

// 与 Go 端 internal/api/dto（V1）一一对应。JSON 使用 snake_case（见 Engine.json）。

@Immutable @Serializable
data class CheckV1(
    val label: String = "",
    val skillName: String = "",
    val roll: Int = 0,
    val modifier: Int = 0,
    val dc: Int = 0,
    val total: Int = 0,
    val success: Boolean = false,
    val critical: Boolean = false,
    val fumble: Boolean = false,
    val explanation: String = "",
)

@Immutable @Serializable
data class QuickActionV1(
    val kind: String = "",
    val action: String? = null,
    val target: String? = null,
    val item: String? = null,
    val destination: String? = null,
    val text: String? = null,
    val label: String? = null,
    val skill: String? = null,
)

@Immutable @Serializable
data class OptionV1(val label: String = "", val action: QuickActionV1 = QuickActionV1())

@Immutable @Serializable
data class EntryV1(
    val id: Long = 0,
    val commandId: String? = null,
    val turn: Int = 0,
    val kind: String = "",
    val text: String = "",
    val check: CheckV1? = null,
    val chips: List<String> = emptyList(),
    val options: List<OptionV1> = emptyList(),
    val corrected: Boolean = false,
    val source: String? = null,
    /** 战斗记录（kind=combat）：掷骰与伤害分解。 */
    val combat: CombatLogV1? = null,
    /** 0.2.0：世界更新（kind=world：知识解锁 / 世界变更 chip）、行动裁定卡（kind=adjudication）、本回合 token 用量。 */
    val world: WorldLogV1? = null,
    val adjudication: AdjudicationV1? = null,
    val usage: UsageV1? = null,
)

@Immutable @Serializable
data class SuggestionV1(
    val label: String = "",
    val icon: String = "",
    val hint: String? = null,
    val disabled: Boolean = false,
    val action: QuickActionV1 = QuickActionV1(),
)

@Immutable @Serializable
data class NPCBriefV1(val id: String = "", val name: String = "", val role: String = "", val attitude: String = "", val portrait: Boolean = false)

@Immutable @Serializable
data class ExitV1(val id: String = "", val label: String = "", val locked: Boolean = false)

@Immutable @Serializable
data class StoryBriefV1(val id: String = "", val title: String = "", val hints: List<String> = emptyList())

@Immutable @Serializable
data class SceneV1(
    val slotId: String = "",
    val saveName: String = "",
    val locationId: String = "",
    val locationName: String = "",
    val description: String = "",
    val timeText: String = "",
    val clock: String = "",
    val day: Int = 1,
    val period: String = "",
    val turn: Int = 0,
    val gold: Int = 0,
    val playerName: String = "",
    val present: List<NPCBriefV1> = emptyList(),
    val exits: List<ExitV1> = emptyList(),
    val facts: List<String> = emptyList(),
    val conditions: List<String> = emptyList(),
    val story: StoryBriefV1? = null,
    /** 故事包声明的实时状态（HUD），每回合由引擎重新计算。 */
    val hud: List<HudFieldV1> = emptyList(),
    val packId: String = "",
    val packName: String = "",
    /** 数值 RPG：当前战斗（null 表示不在战斗中）、是否带数值系统。 */
    val combat: CombatV1? = null,
    val hasRpg: Boolean = false,
    /** 0.2.0：待决 / 仅通知的偏离提示（待决时输入禁用）、状态条上的世界事件倒计时、当前分支、
     *  已选择“回到这里”的回合（>0：继续行动将创建新分支；-1 表示回合 0）、AI 建议。 */
    val decision: DecisionV1? = null,
    val upcoming: List<WorldEventChipV1> = emptyList(),
    val branch: String = "",
    val pendingTurn: Int = 0,
    val aiSuggestions: List<String> = emptyList(),
)

/** HUD 字段。progress 为 0-1000 的千分比，-1 表示不是进度条；tone：normal / success / warning / danger。 */
@Immutable @Serializable
data class HudFieldV1(
    val id: String = "",
    val label: String = "",
    val icon: String = "",
    val value: String = "",
    val compact: Boolean = true,
    val wide: Boolean = false,
    val tone: String = "",
    val progress: Int = -1,
)

/** 故事包卡片（故事包选择界面）。cover 为 base64 编码的封面图片。 */
@Immutable @Serializable
data class PackV1(
    val id: String = "",
    val name: String = "",
    val version: String = "",
    val type: String = "",
    val author: String = "",
    val tagline: String = "",
    val description: String = "",
    val tags: List<String> = emptyList(),
    val icon: String = "",
    val accent: String = "",
    val cover: String = "",
    val engine: String = "",
    val builtin: Boolean = false,
    val playable: Boolean = true,
    val error: String? = null,
    val saveCount: Int = 0,
    val isDefault: Boolean = false,
)

@Immutable @Serializable
data class ImportResultV1(val pack: PackV1 = PackV1(), val replaced: Boolean = false, val previousVersion: String? = null)

@Immutable @Serializable
data class TurnV1(
    val commandId: String = "",
    val accepted: Boolean = false,
    val duplicate: Boolean = false,
    val entries: List<EntryV1> = emptyList(),
    val scene: SceneV1 = SceneV1(),
    val suggestions: List<SuggestionV1> = emptyList(),
    val resolver: String? = null,
    val notices: List<NoticeV1> = emptyList(),
    val usage: UsageV1? = null,
)

@Immutable @Serializable
data class GameBundle(
    val scene: SceneV1 = SceneV1(),
    val transcript: List<EntryV1> = emptyList(),
    val suggestions: List<SuggestionV1> = emptyList(),
)

@Immutable @Serializable
data class StatV1(val id: String = "", val name: String = "", val value: Int = 0, val modifier: Int = 0, val note: String? = null)

@Immutable @Serializable
data class CharacterV1(
    val name: String = "",
    val role: String = "",
    val gold: Int = 0,
    val attributes: List<StatV1> = emptyList(),
    val skills: List<StatV1> = emptyList(),
    val conditions: List<StatV1> = emptyList(),
    val growth: GrowthV1? = null,
    val portrait: Boolean = false,
)

@Immutable @Serializable
data class ItemV1(
    val id: String = "",
    val name: String = "",
    val qty: Int = 0,
    val price: Int = 0,
    val description: String = "",
    val affordable: Boolean = false,
    val use: QuickActionV1? = null,
    val useLabel: String? = null,
    val buy: QuickActionV1? = null,
    val card: CardV1? = null,
    val equipped: Boolean = false,
    val equip: QuickActionV1? = null,
    /** 非空表示机甲改装件 / 挂载武器：点开跳到该机甲卡安装。 */
    val mech: String? = null,
)

@Immutable @Serializable
data class InventoryV1(
    val gold: Int = 0,
    val items: List<ItemV1> = emptyList(),
    val shop: List<ItemV1> = emptyList(),
    val shopSeller: String? = null,
)

@Immutable @Serializable
data class BeliefV1(val text: String = "", val source: String = "", val `when`: String = "", val confidence: Int = 0)

@Immutable @Serializable
data class NPCActionV1(val label: String = "", val chance: Int = -1, val action: QuickActionV1 = QuickActionV1())

@Immutable @Serializable
data class NPCV1(
    val id: String = "",
    val name: String = "",
    val role: String = "",
    val description: String = "",
    val locationName: String = "",
    val present: Boolean = false,
    val trust: Int = 0,
    val fear: Int = 0,
    val attitude: String = "",
    val beliefs: List<BeliefV1> = emptyList(),
    val actions: List<NPCActionV1> = emptyList(),
    /** 和玩家交谈过的次数与 NPC 对玩家的记忆（新的在前）。 */
    val talks: Int = 0,
    val memories: List<String> = emptyList(),
    val portrait: Boolean = false,
)

@Immutable @Serializable
data class JournalEntryV1(val seq: Long = 0, val turn: Int = 0, val time: String = "", val kind: String = "", val text: String = "")

@Immutable @Serializable
data class SlotV1(
    val id: String = "",
    val name: String = "",
    val playerName: String = "",
    val location: String = "",
    val time: String = "",
    val turn: Int = 0,
    val gold: Int = 0,
    val story: String? = null,
    val updatedAt: Long = 0,
    val createdAt: Long = 0,
    val current: Boolean = false,
    /** 存档绑定的故事包；packProblem 非空表示故事包缺失或不兼容（无法读取）。 */
    val packId: String = "",
    val packName: String = "",
    val packVersion: String = "",
    val packProblem: String? = null,
)

@Immutable @Serializable
data class AIStatusV1(
    val kind: String = "offline",
    val baseUrl: String = "",
    val model: String = "",
    val hasKey: Boolean = false,
    val online: Boolean = false,
    val lastError: String? = null,
    val provider: String? = null,
    val mode: String? = null,
    val localWarnings: List<String> = emptyList(),
)

@Immutable @Serializable
data class PresetV1(val kind: String = "", val label: String = "", val baseUrl: String = "", val model: String = "")

@Serializable
data class AIConfig(val kind: String, val baseUrl: String = "", val model: String = "", val apiKey: String = "")

@Serializable
data class TestAIResult(val ok: Boolean = false, val reply: String = "", val latencyMs: Long = 0, val error: String? = null)

@Serializable
data class StreamEventV1(
    val version: String = "",
    val type: String = "",
    val commandId: String = "",
    val text: String? = null,
    val corrected: Boolean = false,
    val source: String? = null,
)
