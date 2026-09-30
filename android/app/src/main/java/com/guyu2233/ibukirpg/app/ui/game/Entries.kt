package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AutoStories
import androidx.compose.material.icons.outlined.Casino
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Verified
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.CheckV1
import com.guyu2233.ibukirpg.app.data.EntryV1
import com.guyu2233.ibukirpg.app.data.OptionV1
import com.guyu2233.ibukirpg.app.ui.theme.NarrativeStyle
import kotlinx.coroutines.delay
import kotlin.math.min

@Composable
fun EntryItem(
    entry: EntryV1,
    animate: Boolean,
    onAnimated: (Long) -> Unit,
    onOption: (OptionV1) -> Unit,
    enabled: Boolean,
    onCard: (String) -> Unit = {},
) {
    when (entry.kind) {
        "combat" -> CombatEntry(entry, onCard)
        "player" -> PlayerBubble(entry.text)
        "narration", "intro" -> Narration(entry, animate, onAnimated)
        "check" -> entry.check?.let { CheckCard(it) }
        "story" -> StoryBanner(entry.text)
        else -> SystemEntry(entry, onOption, enabled)
    }
}

@Composable
fun PlayerBubble(text: String, sending: Boolean = false) {
    val you = stringResource(R.string.game_you)
    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.CenterEnd) {
        Surface(
            color = MaterialTheme.colorScheme.primaryContainer,
            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
            shape = RoundedCornerShape(topStart = 20.dp, topEnd = 6.dp, bottomStart = 20.dp, bottomEnd = 20.dp),
            modifier = Modifier
                .widthIn(max = 320.dp)
                .padding(start = 48.dp)
                .semantics { contentDescription = "$you：$text" },
        ) {
            Text(
                text, style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                color = if (sending) MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.7f) else MaterialTheme.colorScheme.onPrimaryContainer,
            )
        }
    }
}

@Composable
private fun Narration(entry: EntryV1, animate: Boolean, onAnimated: (Long) -> Unit) {
    Column(Modifier.fillMaxWidth()) {
        TypewriterText(entry.text, animate) { onAnimated(entry.id) }
        if (entry.corrected) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 4.dp)) {
                Icon(Icons.Outlined.Verified, contentDescription = null, modifier = Modifier.size(14.dp), tint = MaterialTheme.colorScheme.outline)
                Spacer(Modifier.size(4.dp))
                Text(stringResource(R.string.game_corrected), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            }
        }
    }
}

/** 逐字显示新到达的叙事；点击立即显示全文。读屏软件始终读到全文。 */
@Composable
fun TypewriterText(text: String, animate: Boolean, onDone: () -> Unit) {
    var shown by remember(text) { mutableIntStateOf(if (animate) 0 else text.length) }
    LaunchedEffect(text, animate) {
        while (shown < text.length) {
            delay(16)
            shown = min(text.length, shown + 2)
        }
        if (animate) onDone()
    }
    Text(
        text.substring(0, shown),
        style = NarrativeStyle,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = shown < text.length) { shown = text.length }
            .clearAndSetSemantics { contentDescription = text },
    )
}

@Composable
fun StreamingNarration(text: String) {
    Text(text, style = NarrativeStyle, color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.fillMaxWidth())
}

@Composable
fun CheckCard(c: CheckV1) {
    val ok = c.success
    val container = if (ok) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.errorContainer
    val onContainer = if (ok) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onErrorContainer
    val verdict = when {
        c.critical -> stringResource(R.string.game_check_critical)
        c.fumble -> stringResource(R.string.game_check_fumble)
        ok -> stringResource(R.string.game_check_success)
        else -> stringResource(R.string.game_check_failure)
    }
    val sign = if (c.modifier >= 0) "+" else "−"
    val skill = c.skillName.ifBlank { "修正" }
    val math = "d20 掷出 ${c.roll}  $sign  $skill ${kotlin.math.abs(c.modifier)}  =  ${c.total}   （需要 ≥ ${c.dc}）"
    Surface(
        color = container.copy(alpha = 0.55f),
        contentColor = onContainer,
        shape = RoundedCornerShape(14.dp),
        border = BorderStroke(1.dp, container),
        modifier = Modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) {},
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.Casino, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.size(8.dp))
                Text(
                    c.label.ifBlank { c.skillName } + "检定",
                    style = MaterialTheme.typography.labelLarge, modifier = Modifier.weight(1f),
                )
                Surface(color = container, shape = RoundedCornerShape(8.dp)) {
                    Text(
                        verdict, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
                    )
                }
            }
            Spacer(Modifier.height(4.dp))
            Text(math, style = MaterialTheme.typography.labelMedium)
            // 普通检定的算式已经说明一切；天然 20 / 1 时额外显示解释。
            if (c.explanation.isNotBlank() && (c.critical || c.fumble)) {
                Text(
                    c.explanation, style = MaterialTheme.typography.bodySmall,
                    color = LocalContentColor.current.copy(alpha = 0.85f),
                )
            }
        }
    }
}

@Composable
private fun StoryBanner(text: String) {
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = RoundedCornerShape(14.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.AutoStories, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.size(8.dp))
            Text(text, style = MaterialTheme.typography.titleSmall)
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SystemEntry(entry: EntryV1, onOption: (OptionV1) -> Unit, enabled: Boolean) {
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        // 带 chips 的系统条目（金币 / 物品 / 关系变化）只显示 chips，避免重复。
        if (entry.text.isNotBlank() && entry.chips.isEmpty()) {
            Surface(
                color = MaterialTheme.colorScheme.surfaceContainerHigh,
                contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                shape = RoundedCornerShape(14.dp),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                    Icon(Icons.Outlined.Info, contentDescription = null, modifier = Modifier.size(18.dp).padding(top = 2.dp))
                    Spacer(Modifier.size(8.dp))
                    Text(entry.text, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
        if (entry.chips.isNotEmpty()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                entry.chips.forEach { ResultChip(it) }
            }
        }
        if (entry.options.isNotEmpty()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                entry.options.forEach { o ->
                    FilledTonalButton(onClick = { onOption(o) }, enabled = enabled) { Text(o.label) }
                }
            }
        }
    }
}

@Composable
private fun ResultChip(text: String) {
    val positive = text.contains("+") || text.startsWith("获得")
    val negative = text.contains("-") || text.contains("−") || text.startsWith("失去")
    val color = when {
        positive -> MaterialTheme.colorScheme.primary
        negative -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Surface(
        shape = RoundedCornerShape(8.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        color = MaterialTheme.colorScheme.surface,
    ) {
        CompositionLocalProvider(LocalContentColor provides color) {
            Text(text, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp))
        }
    }
}

/** 战斗记录：行动者 → 目标、命中掷骰、伤害与分解标签；点按打开技能 / 道具卡。 */
@OptIn(androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun CombatEntry(entry: EntryV1, onCard: (String) -> Unit) {
    val log = entry.combat
    val enemy = log?.side == "enemy"
    val accent = if (enemy) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary
    val ref = log?.skillId ?: log?.itemId
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = androidx.compose.foundation.BorderStroke(1.dp, accent.copy(alpha = 0.35f)),
        modifier = Modifier.fillMaxWidth().then(
            if (ref != null) Modifier.clickable { onCard(ref) } else Modifier,
        ),
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(8.dp).background(accent, RoundedCornerShape(4.dp)))
                Spacer(Modifier.size(8.dp))
                Text(entry.text, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                if (log != null && log.damage > 0) {
                    Text(
                        (if (log.critical) "暴击 " else "") + "-${log.damage}",
                        style = MaterialTheme.typography.titleSmall,
                        color = if (log.critical) MaterialTheme.colorScheme.tertiary else accent,
                    )
                }
            }
            if (log != null && (log.chips.isNotEmpty() || log.chance > 0)) {
                androidx.compose.foundation.layout.FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    modifier = Modifier.padding(top = 4.dp),
                ) {
                    if (log.chance > 0) ResultChip("掷 ${log.roll} / ${log.chance}")
                    log.chips.forEach { ResultChip(it) }
                }
            }
        }
    }
}
