package com.guyu2233.ibukirpg.app.data

import androidx.compose.runtime.Immutable
import kotlinx.serialization.Serializable

// v0.1.2-rc2：数值 RPG（战斗 / 成长 / 图鉴 / 关系网 / 角色卡 / 主线 / 机甲卡）。与 Go 端 internal/api/dto 对应。

@Immutable @Serializable
data class CombatLogV1(
    val actor: String = "",
    val target: String? = null,
    val action: String = "",
    val skillId: String? = null,
    val itemId: String? = null,
    val hit: Boolean = false,
    val roll: Int = 0,
    val chance: Int = 0,
    val damage: Int = 0,
    val critical: Boolean = false,
    val side: String? = null,
    val chips: List<String> = emptyList(),
)

@Immutable @Serializable
data class StatusChipV1(val id: String = "", val name: String = "", val icon: String? = null, val turns: Int = 0, val debuff: Boolean = false, val note: String? = null)

@Immutable @Serializable
data class CombatUnitV1(
    val id: String = "",
    val ref: String = "",
    val name: String = "",
    val side: String = "",
    val level: Int = 0,
    val hp: Int = 0,
    val maxHp: Int = 0,
    val sp: Int = 0,
    val maxSp: Int = 0,
    val mech: Boolean = false,
    val mechName: String? = null,
    val pilotHp: Int = 0,
    val heat: Int = 0,
    val heatMax: Int = 0,
    val statuses: List<StatusChipV1> = emptyList(),
    val down: Boolean = false,
    val current: Boolean = false,
    val defending: Boolean = false,
    val icon: String? = null,
    val tier: String? = null,
    val portrait: Boolean = false,
)

@Immutable @Serializable
data class CombatActionV1(
    val kind: String = "",
    val id: String? = null,
    val label: String = "",
    val icon: String? = null,
    val hint: String? = null,
    val target: String = "none",
    val disabled: Boolean = false,
    val reason: String? = null,
    val qty: Int = 0,
)

@Immutable @Serializable
data class CombatV1(
    val encounter: String = "",
    val title: String = "",
    val round: Int = 0,
    val yourTurn: Boolean = false,
    val party: List<CombatUnitV1> = emptyList(),
    val enemies: List<CombatUnitV1> = emptyList(),
    val actions: List<CombatActionV1> = emptyList(),
    val skills: List<CombatActionV1> = emptyList(),
    val items: List<CombatActionV1> = emptyList(),
    val order: List<String> = emptyList(),
    val resourceName: String = "",
    val mercury: Int = 0,
    val mercuryMax: Int = 0,
)

@Immutable @Serializable
data class NodeV1(
    val id: String = "",
    val title: String = "",
    val objective: String = "",
    val goal: String = "",
    val source: String = "",
    val reward: String? = null,
    val location: String? = null,
)

@Immutable @Serializable
data class MainlineV1(
    val mode: String = "main",
    val modeLabel: String = "",
    val deviation: Int = 0,
    val adherence: Int = 100,
    val mild: Int = 35,
    val heavy: Int = 70,
    val level: Int = 0,
    val pending: Boolean = false,
    val anchor: String? = null,
    val objective: String? = null,
    val node: NodeV1? = null,
    val freeOnline: Boolean = false,
    val anchors: Int = 0,
    val anchorIdx: Int = 0,
)

@Immutable @Serializable
data class NoticeV1(val kind: String = "", val text: String = "", val ref: String? = null)

@Immutable @Serializable
data class KVV1(val label: String = "", val value: String = "")

@Immutable @Serializable
data class CardV1(
    val id: String = "",
    val kind: String = "",
    val kindName: String = "",
    val name: String = "",
    val icon: String? = null,
    val rarity: String? = null,
    val rarityName: String? = null,
    val rarityColor: String? = null,
    val description: String? = null,
    val lore: String? = null,
    val stats: List<KVV1> = emptyList(),
    val tags: List<String> = emptyList(),
    val known: Boolean = false,
)

@Immutable @Serializable
data class CodexCategoryV1(val kind: String = "", val name: String = "", val unlocked: Int = 0, val entries: List<CardV1> = emptyList())

@Immutable @Serializable
data class CodexV1(val categories: List<CodexCategoryV1> = emptyList(), val unlocked: Int = 0, val total: Int = 0)

@Immutable @Serializable
data class DimValueV1(val id: String = "", val name: String = "", val value: Int = 0, val negative: Boolean = false)

@Immutable @Serializable
data class EdgeV1(
    val from: String = "",
    val to: String = "",
    val fromName: String = "",
    val toName: String = "",
    val values: List<DimValueV1> = emptyList(),
    val label: String = "",
    val tone: String = "neutral",
    val source: String = "",
    val turn: Int = 0,
    val note: String? = null,
)

@Immutable @Serializable
data class PersonV1(
    val id: String = "",
    val name: String = "",
    val role: String? = null,
    val icon: String? = null,
    val card: String? = null,
    val player: Boolean = false,
    val portrait: Boolean = false,
)

@Immutable @Serializable
data class RelChangeV1(val turn: Int = 0, val time: String = "", val from: String = "", val to: String = "", val text: String = "")

@Immutable @Serializable
data class RelationsV1(
    val dimensions: List<DimValueV1> = emptyList(),
    val people: List<PersonV1> = emptyList(),
    val edges: List<EdgeV1> = emptyList(),
    val history: List<RelChangeV1> = emptyList(),
)

@Immutable @Serializable
data class CharacterCardV1(
    val id: String = "",
    val name: String = "",
    val role: String? = null,
    val icon: String? = null,
    val description: String? = null,
    val lore: String? = null,
    val status: String = "active",
    val reason: String? = null,
    val faction: String? = null,
    val level: Int = 0,
    val stats: List<StatV1> = emptyList(),
    val relation: List<DimValueV1> = emptyList(),
    val memories: List<String> = emptyList(),
    val location: String? = null,
    val dynamic: Boolean = false,
    val turn: Int = 0,
    val portrait: Boolean = false,
    val mechs: List<String> = emptyList(),
)

@Immutable @Serializable
data class CardsV1(
    val active: List<CharacterCardV1> = emptyList(),
    val archived: List<CharacterCardV1> = emptyList(),
    val dead: List<CharacterCardV1> = emptyList(),
)

@Immutable @Serializable
data class EquipSlotV1(val slot: String = "", val name: String = "", val item: CardV1? = null, val unequip: QuickActionV1? = null)

@Immutable @Serializable
data class SkillCardV1(val card: CardV1 = CardV1(), val learned: Boolean = false, val learn: QuickActionV1? = null, val blocked: String? = null, val cost: Int = 0)

@Immutable @Serializable
data class AttributeV1(val id: String = "", val name: String = "", val value: Int = 0, val effect: String? = null, val allocate: QuickActionV1? = null)

@Immutable @Serializable
data class GrowthV1(
    val level: Int = 1,
    val xp: Int = 0,
    val xpNext: Int = 100,
    val attrPoints: Int = 0,
    val skillPoints: Int = 0,
    val hp: Int = 0,
    val maxHp: Int = 0,
    val mercury: Int = 0,
    val mercuryMax: Int = 0,
    val resourceName: String = "",
    val hasMech: Boolean = false,
    val mechName: String? = null,
    val stats: List<StatV1> = emptyList(),
    val equipment: List<EquipSlotV1> = emptyList(),
    val skills: List<SkillCardV1> = emptyList(),
    val attributes: List<AttributeV1> = emptyList(),
)

// ---------- 机械甲胄卡 ----------

@Immutable @Serializable
data class MechFieldV1(val id: String = "", val label: String = "", val value: String = "", val known: Boolean = false)

@Immutable @Serializable
data class MechStatV1(
    val id: String = "",
    val name: String = "",
    val value: Int = 0,
    val max: Int = 100,
    val unit: String? = null,
    val bonus: Int = 0,
    val known: Boolean = false,
)

@Immutable @Serializable
data class MechStatusV1(val id: String = "", val name: String = "", val tone: String = "neutral", val engage: Boolean = false, val known: Boolean = false)

@Immutable @Serializable
data class MechSlotV1(
    val id: String = "",
    val name: String = "",
    val kind: String? = null,
    val hardpoint: Boolean = false,
    val known: Boolean = false,
    val part: CardV1? = null,
    val remove: QuickActionV1? = null,
    val install: List<QuickActionV1> = emptyList(),
)

@Immutable @Serializable
data class MechLogV1(val turn: Int = 0, val time: String? = null, val text: String = "")

@Immutable @Serializable
data class MechCardV1(
    val id: String = "",
    val name: String = "",
    val icon: String? = null,
    val rarity: String? = null,
    val rarityName: String? = null,
    val rarityColor: String? = null,
    val known: Boolean = false,
    val owned: Boolean = false,
    val enemy: Boolean = false,
    val portrait: Boolean = false,
    val status: MechStatusV1 = MechStatusV1(),
    val fields: List<MechFieldV1> = emptyList(),
    val pilotId: String? = null,
    val specs: List<MechStatV1> = emptyList(),
    val stats: List<MechStatV1> = emptyList(),
    val statsKnown: Boolean = false,
    val energy: List<MechFieldV1> = emptyList(),
    val slots: List<MechSlotV1> = emptyList(),
    val hardpoints: List<MechSlotV1> = emptyList(),
    val skills: List<CardV1> = emptyList(),
    val description: String? = null,
    val lore: String? = null,
    val loreKnown: Boolean = false,
    val history: List<MechLogV1> = emptyList(),
    val unknown: Int = 0,
)

@Immutable @Serializable
data class MechsV1(val mechs: List<MechCardV1> = emptyList(), val resourceName: String = "")

@Immutable @Serializable
data class PortraitV1(val id: String = "", val mime: String = "", val base64: String = "")
