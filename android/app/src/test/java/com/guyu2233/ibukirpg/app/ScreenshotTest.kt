package com.guyu2233.ibukirpg.app

import android.app.Application
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.ui.Alignment
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.GenSettingsV1
import com.guyu2233.ibukirpg.app.data.PromptSettingsV1
import com.guyu2233.ibukirpg.app.data.ProviderV1
import com.guyu2233.ibukirpg.app.data.RouteV1
import com.guyu2233.ibukirpg.app.data.SensitivityV1
import com.guyu2233.ibukirpg.app.data.TasksV1
import com.guyu2233.ibukirpg.app.data.TimelineV1
import com.guyu2233.ibukirpg.app.data.WorldPanelV1
import com.guyu2233.ibukirpg.app.ui.game.DecisionContent
import com.guyu2233.ibukirpg.app.ui.game.SensitivityContent
import com.guyu2233.ibukirpg.app.ui.game.TimelineActions
import com.guyu2233.ibukirpg.app.ui.game.TimelineContent
import com.guyu2233.ibukirpg.app.ui.game.WorldPanelContent
import com.guyu2233.ibukirpg.app.ui.game.WorldState
import com.guyu2233.ibukirpg.app.ui.game.WorldTab
import com.guyu2233.ibukirpg.app.ui.settings.AiSettingsState
import com.guyu2233.ibukirpg.app.ui.settings.GenerationSettingsContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import com.guyu2233.ibukirpg.app.data.AIStatusV1
import com.guyu2233.ibukirpg.app.data.AppSettings
import com.guyu2233.ibukirpg.app.data.CharacterV1
import com.guyu2233.ibukirpg.app.data.GameBundle
import com.guyu2233.ibukirpg.app.data.InventoryV1
import com.guyu2233.ibukirpg.app.data.JournalEntryV1
import com.guyu2233.ibukirpg.app.data.NPCV1
import com.guyu2233.ibukirpg.app.data.PackV1
import com.guyu2233.ibukirpg.app.data.SlotV1
import com.guyu2233.ibukirpg.app.data.ThemeMode
import com.guyu2233.ibukirpg.app.ui.game.GameContent
import com.guyu2233.ibukirpg.app.ui.game.GameState
import com.guyu2233.ibukirpg.app.ui.game.PanelsContent
import com.guyu2233.ibukirpg.app.ui.game.PanelsState
import com.guyu2233.ibukirpg.app.ui.home.HomeContent
import com.guyu2233.ibukirpg.app.ui.home.HomeState
import com.guyu2233.ibukirpg.app.ui.packs.PacksContent
import com.guyu2233.ibukirpg.app.ui.packs.PacksState
import com.guyu2233.ibukirpg.app.ui.saves.SavesContent
import com.guyu2233.ibukirpg.app.ui.saves.SavesState
import com.guyu2233.ibukirpg.app.ui.settings.AIForm
import com.guyu2233.ibukirpg.app.ui.settings.SettingsActions
import com.guyu2233.ibukirpg.app.ui.settings.SettingsContent
import com.guyu2233.ibukirpg.app.ui.settings.TestState
import com.guyu2233.ibukirpg.app.ui.theme.IbukiTheme
import com.guyu2233.ibukirpg.app.data.CardsV1
import com.guyu2233.ibukirpg.app.data.CodexV1
import com.guyu2233.ibukirpg.app.data.MechCardV1
import com.guyu2233.ibukirpg.app.data.MechsV1
import com.guyu2233.ibukirpg.app.data.PortraitV1
import com.guyu2233.ibukirpg.app.data.RelationsV1
import com.guyu2233.ibukirpg.app.ui.rpg.CardDialog
import com.guyu2233.ibukirpg.app.ui.rpg.LocalRpgData
import com.guyu2233.ibukirpg.app.ui.rpg.MechCardScreen
import com.guyu2233.ibukirpg.app.ui.rpg.RpgData
import androidx.compose.runtime.CompositionLocalProvider
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNamingStrategy
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.io.File

/**
 * JVM 截图测试：用 Robolectric（原生图形模式）渲染真实的界面 Composable。
 * 游戏数据来自 `go run ./cmd/uifixtures` —— 真实 Go 引擎离线试玩 5 回合后导出的 JSON（含故事包列表与 HUD）。
 * 输出目录：<repo>/build/screenshots
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], application = Application::class, qualifiers = "w411dp-h891dp-xxhdpi")
class ScreenshotTest {
    @get:Rule val compose = createComposeRule()

    @OptIn(ExperimentalSerializationApi::class)
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
        namingStrategy = JsonNamingStrategy.SnakeCase
    }

    /** 夹具：优先读 -Pibuki.fixtures.dir 指定的目录（本地私有包截图），否则读测试资源。 */
    private inline fun <reified T> fixture(name: String): T {
        val local = System.getProperty("ibuki.fixtures.dir")?.let { File(it, name) }?.takeIf { it.exists() }
        val text = local?.readText() ?: javaClass.classLoader!!.getResource("fixtures/$name")!!.readText()
        return json.decodeFromString(text)
    }

    private fun out(name: String): String {
        val dir = File(System.getProperty("ibuki.shots.dir") ?: "build/screenshots").apply { mkdirs() }
        return File(dir, name).absolutePath
    }

    private fun shoot(name: String, dark: Boolean = false, content: @Composable () -> Unit) {
        val data = rpgData
        compose.setContent {
            IbukiTheme(theme = if (dark) ThemeMode.DARK else ThemeMode.LIGHT, dynamicColor = false) {
                CompositionLocalProvider(LocalRpgData provides data) {
                    Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) { content() }
                }
            }
        }
        compose.waitForIdle()
        compose.onRoot().captureRoboImage(out(name))
    }

    private val slots: List<SlotV1> get() = fixture("saves.json")

    private val game: GameState get() {
        val b: GameBundle = fixture("game.json")
        return GameState(loading = false, scene = b.scene, entries = b.transcript, suggestions = b.suggestions)
    }

    private val panels: PanelsState get() = PanelsState(
        character = fixture<CharacterV1>("character.json"),
        inventory = fixture<InventoryV1>("inventory.json"),
        npcs = fixture<List<NPCV1>>("npcs.json"),
        journal = fixture<List<JournalEntryV1>>("journal.json"),
    )

    // ---------- 数值 RPG（内置示例包“锈钟镇·黄铜试炼”真实试玩导出） ----------

    private val rpgMechs: MechsV1 get() = fixture("rpg_mechs.json")
    private val rpgCodex: CodexV1 get() = fixture("rpg_codex.json")

    private val rpgData: RpgData by lazy {
        val portraits: Map<String, PortraitV1> = runCatching { fixture<Map<String, PortraitV1>>("rpg_portraits.json") }.getOrDefault(emptyMap())
        val mechs = runCatching { rpgMechs.mechs }.getOrDefault(emptyList())
        val cards = runCatching { rpgCodex.categories.flatMap { it.entries } }.getOrDefault(emptyList())
        RpgData(
            portrait = { id -> portraits[id]?.let { java.util.Base64.getMimeDecoder().decode(it.base64) } },
            card = { id -> cards.firstOrNull { it.id == id } },
            mech = { id -> mechs.firstOrNull { it.id == id } },
        )
    }

    private fun rpgGame(name: String): GameState {
        val b: GameBundle = fixture(name)
        return GameState(loading = false, scene = b.scene, entries = b.transcript, suggestions = b.suggestions)
    }

    private val rpgPanels: PanelsState get() = PanelsState(
        character = fixture<CharacterV1>("rpg_character.json"),
        inventory = fixture<InventoryV1>("rpg_inventory.json"),
        npcs = fixture<List<NPCV1>>("rpg_npcs.json"),
        journal = fixture<List<JournalEntryV1>>("rpg_journal.json"),
        codex = rpgCodex,
        relations = fixture<RelationsV1>("rpg_relations.json"),
        cards = fixture<CardsV1>("rpg_cards.json"),
        mechs = rpgMechs,
    )

    /** 截图用的机甲：优先选己方且信息最全的。 */
    private val showcaseMech: MechCardV1 get() = rpgMechs.mechs.sortedWith(compareByDescending<MechCardV1> { it.known }.thenByDescending { it.owned }.thenBy { it.unknown }).first()

    @Test fun rpgCombat() = shoot("15-rpg-combat.png") {
        GameContent(rpgGame("rpg_combat.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun rpgCombatDark() = shoot("16-rpg-combat-dark.png", dark = true) {
        GameContent(rpgGame("rpg_combat.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun rpgAfterBattle() = shoot("17-rpg-game.png") {
        GameContent(rpgGame("rpg_game.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, initialHudExpanded = true)
    }

    @Test fun rpgGrowth() = shoot("18-rpg-character.png") {
        PanelsContent(rpgPanels, busy = false, onAction = { _, _ -> }, initialTab = 0)
    }

    @Test fun rpgInventory() = shoot("19-rpg-inventory.png") {
        PanelsContent(rpgPanels, busy = false, onAction = { _, _ -> }, initialTab = 1)
    }

    @Test fun rpgPeople() = shoot("20-rpg-people.png") {
        PanelsContent(rpgPanels, busy = false, onAction = { _, _ -> }, initialTab = 2)
    }

    @Test fun rpgRelations() = shoot("21-rpg-relations.png") {
        PanelsContent(rpgPanels, busy = false, onAction = { _, _ -> }, initialTab = 3)
    }

    @Test fun rpgCodex() = shoot("22-rpg-codex.png") {
        PanelsContent(rpgPanels, busy = false, onAction = { _, _ -> }, initialTab = 4)
    }

    @Test fun rpgMechCard() = shoot("23-rpg-mech.png") {
        MechCardScreen(showcaseMech, onBack = {}, onAction = {})
    }

    @Test fun rpgMechCardDark() = shoot("24-rpg-mech-dark.png", dark = true) {
        MechCardScreen(rpgMechs.mechs.maxBy { it.unknown }, onBack = {}, onAction = {})
    }

    @Test fun rpgCardDialog() = shoot("25-rpg-card.png") {
        val c = rpgCodex.categories.flatMap { it.entries }.filter { it.known }.maxBy { it.stats.size + (it.lore?.length ?: 0) / 20 }
        CardDialog(c, onDismiss = {})
    }

    /** 内置故事包 + 一个模拟的“已导入”故事包 + 一个不兼容的导入包（展示错误状态）。 */
    private val packs: PacksState get() {
        val builtin: List<PackV1> = fixture("packs.json")
        val imported = PackV1(
            id = "moon_well", name = "月井奇谭", version = "1.0.0", type = "story", author = "玩家投稿",
            tagline = "井底的月亮会说话。", description = "一口古井，一个只在满月出现的倒影。社区制作的示例故事包。",
            tags = listOf("悬疑", "社区"), icon = "star", accent = "#4A5AA8", builtin = false, playable = true,
        )
        val broken = PackV1(
            id = "future_pack", name = "星海远航", version = "2.0.0", type = "story", author = "某作者",
            tagline = "需要更新的引擎版本。", builtin = false, playable = false,
            error = "需要引擎版本 >=0.3.0，当前是 0.2.0-alpha1。请先更新 App。", accent = "#37474F",
        )
        return PacksState(loading = false, packs = builtin + imported + broken)
    }

    private val home: HomeState get() {
        val s = slots
        return HomeState(loading = false, latest = s.firstOrNull(), saveCount = s.size, ai = AIStatusV1(kind = "offline"), version = "0.2.0-alpha1")
    }

    @Test fun home() = shoot("01-home.png") {
        HomeContent(home, {}, {}, {}, {}, {})
    }

    @Test fun homeDark() = shoot("02-home-dark.png", dark = true) {
        HomeContent(home, {}, {}, {}, {}, {})
    }

    @Test fun gameScreen() = shoot("03-game.png") {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun gameDarkTyping() = shoot("04-game-dark.png", dark = true) {
        GameContent(game, "我想问问米拉刚才看见了什么", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun panelsPeople() = shoot("05-panel-people.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 2)
    }

    @Test fun panelsInventory() = shoot("06-panel-inventory.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 1)
    }

    @Test fun panelsCharacter() = shoot("07-panel-character.png") {
        PanelsContent(panels, busy = false, onAction = { _, _ -> }, initialTab = 0)
    }

    @Test fun packPicker() = shoot("10-packs.png") {
        PacksContent(packs, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun packPickerDark() = shoot("11-packs-dark.png", dark = true) {
        PacksContent(packs.copy(packs = packs.packs.drop(1)), {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun gameHudExpanded() = shoot("12-game-hud-expanded.png") {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, initialHudExpanded = true)
    }

    @Test fun gameHudDarkExpanded() = shoot("13-game-hud-dark.png", dark = true) {
        GameContent(game, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, initialHudExpanded = true)
    }

    @Test fun saves() = shoot("14-saves.png") {
        val list = slots + slots.last().copy(
            id = "missing", name = "月井奇谭 · 旧存档", packId = "moon_well", packName = "月井奇谭", packVersion = "1.0.0",
            location = "古井边", story = null, current = false, packProblem = "这个存档使用的故事包 \"moon_well\"（1.0.0）没有安装或已被删除。重新导入该故事包后即可继续。",
        )
        SavesContent(SavesState(loading = false, slots = list), {}, {}, { _, _ -> }, { _, _ -> }, { _, _ -> }, {})
    }

    @Test fun settings() = shoot("08-settings.png") {
        SettingsContent(
            settings = AppSettings(aiKind = "deepseek"),
            form = AIForm(kind = "deepseek", baseUrl = "https://api.deepseek.com", model = "deepseek-chat", hasSavedKey = true, test = TestState.Idle),
            saved = false,
            engineVersion = "0.2.0-alpha1",
            actions = SettingsActions(),
        )
    }

    @Test fun settingsOffline() = shoot("09-settings-offline.png") {
        SettingsContent(
            settings = AppSettings(),
            form = AIForm(kind = "offline"),
            saved = false,
            engineVersion = "0.2.0-alpha1",
            actions = SettingsActions(),
        )
    }

    // ---------- 0.2.0 开放世界（假 AI 驱动真实引擎导出的夹具：cmd/uifixtures/world.go） ----------

    private val worldTabs: WorldState get() = WorldState(
        tabs = WorldTab.entries.associate { it.id to fixture<WorldPanelV1>("v02_world_${it.id}.json") },
    )

    /** 截图里的“弹出层”：在界面上叠一层遮罩 + 底部圆角面板（Robolectric 截不到真正的弹窗窗口）。 */
    @Composable
    private fun SheetOver(background: @Composable () -> Unit, heightFraction: Float = 0f, sheet: @Composable () -> Unit) {
        Box(Modifier.fillMaxSize()) {
            background()
            Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.32f)))
            Surface(
                shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
                color = MaterialTheme.colorScheme.surfaceContainerLow,
                modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth().let { if (heightFraction > 0f) it.fillMaxHeight(heightFraction) else it },
            ) {
                Column {
                    Box(Modifier.fillMaxWidth().padding(top = 12.dp, bottom = 8.dp), contentAlignment = Alignment.Center) {
                        Box(Modifier.size(width = 32.dp, height = 4.dp).background(MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f), RoundedCornerShape(2.dp)))
                    }
                    sheet()
                }
            }
        }
    }

    @Test fun v02Game() = shoot("30-v02-game.png") {
        GameContent(rpgGame("v02_game.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun v02GameDark() = shoot("31-v02-game-dark.png", dark = true) {
        GameContent(rpgGame("v02_game.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun v02Combat() = shoot("32-v02-combat.png") {
        GameContent(rpgGame("v02_combat.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {})
    }

    @Test fun v02Decision() = shoot("33-v02-decision.png") {
        val g = rpgGame("v02_decision.json")
        SheetOver({ GameContent(g, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, showDecisionSheet = false) }) {
            DecisionContent(g.scene.decision!!, busy = false, onAccept = {}, onRollback = {}, onSensitivity = {})
        }
    }

    @Test fun v02Timeline() = shoot("34-v02-timeline.png") {
        val g = rpgGame("v02_after.json")
        TimelineContent(fixture<TimelineV1>("v02_timeline.json"), g.scene.saveName.ifBlank { "阿砾" }, g.scene.packName, TimelineActions(), initialSelected = 14)
    }

    @Test fun v02WorldCharacters() = worldShot("35-v02-world-characters.png", 0)
    @Test fun v02WorldRelations() = worldShot("36-v02-world-relations.png", 1)
    @Test fun v02WorldFactions() = worldShot("37-v02-world-factions.png", 5)
    @Test fun v02WorldTimeline() = worldShot("38-v02-world-timeline.png", 6)
    @Test fun v02WorldLog() = worldShot("39-v02-world-log.png", 7)

    private fun worldShot(name: String, tab: Int) = shoot(name) {
        val g = rpgGame("v02_after.json")
        SheetOver({ GameContent(g, "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, showDecisionSheet = false) }, heightFraction = 0.9f) {
            WorldPanelContent(worldTabs, busy = false, onTab = {}, onAction = { _, _ -> }, onRevert = {}, initialTab = tab)
        }
    }

    @Test
    @Config(qualifiers = "w1280dp-h800dp-xhdpi")
    fun v02Tablet() = shoot("40-v02-tablet.png") {
        GameContent(rpgGame("v02_after.json"), "", PanelsState(), {}, {}, {}, { _, _ -> }, {}, {}, {}, world = worldTabs, initialWorld = true, showDecisionSheet = false)
    }

    private val aiState: AiSettingsState get() = AiSettingsState(
        providers = listOf(
            ProviderV1(id = "deepseek", kind = "deepseek", label = "DeepSeek", model = "deepseek-chat"),
            ProviderV1(id = "qwen", kind = "qwen", label = "通义千问", model = "qwen-plus"),
            ProviderV1(id = "local", kind = "llamacpp", label = "本地", model = "Qwen2.5-3B Q4_K_M", sizeB = 3.0),
        ),
        keys = setOf("deepseek", "qwen"),
        gen = GenSettingsV1(
            mode = "per_task", unified = RouteV1("deepseek"),
            tasks = mapOf(
                "narrate_world" to RouteV1("deepseek"), "parse_action" to RouteV1("local"),
                "combat_adjudicate" to RouteV1("qwen"), "memory" to RouteV1("local"), "audit" to RouteV1("deepseek", "deepseek-reasoner"),
            ),
        ),
        tasks = fixture<TasksV1>("v02_tasks.json").tasks,
    )

    @Test fun v02Generation() = shoot("41-v02-generation.png") {
        GenerationSettingsContent(aiState, onGen = {}, onClose = {})
    }

    @Test fun v02GenerationLocalFit() = shoot("42-v02-generation-local-fit.png") {
        GenerationSettingsContent(aiState, onGen = {}, onClose = {}, initialFitSheet = true, inlineFitSheet = true)
    }

    @Test fun v02Settings() = shoot("43-v02-settings.png") {
        SettingsContent(
            settings = AppSettings(aiKind = "deepseek"),
            form = AIForm(kind = "deepseek", baseUrl = "https://api.deepseek.com", model = "deepseek-chat", hasSavedKey = true, test = TestState.Idle),
            saved = false,
            engineVersion = "0.2.0-alpha1",
            actions = SettingsActions(),
            ai = aiState,
        )
    }

    @Test fun v02Sensitivity() = shoot("44-v02-sensitivity.png") {
        Surface(Modifier.padding(16.dp), shape = RoundedCornerShape(28.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            Column(Modifier.padding(24.dp)) {
                Text("提示灵敏度", style = MaterialTheme.typography.headlineSmall)
                Spacer(Modifier.height(16.dp))
                SensitivityContent(PromptSettingsV1(majorDeath = SensitivityV1("high", "modal"), storyImpact = SensitivityV1("low", "notify"))) {}
            }
        }
    }
}
