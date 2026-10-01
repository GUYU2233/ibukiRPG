package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Lightbulb
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.HudFieldV1
import com.guyu2233.ibukirpg.app.data.StoryBriefV1
import com.guyu2233.ibukirpg.app.data.WorldEventChipV1
import androidx.compose.material.icons.outlined.Campaign
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material.icons.outlined.Healing
import com.guyu2233.ibukirpg.app.ui.common.symbolFor

/** HUD 字段的色调 → (容器色, 内容色)。 */
@Composable
private fun toneColors(tone: String): Pair<Color, Color> {
    val c = MaterialTheme.colorScheme
    return when (tone) {
        "danger" -> c.errorContainer to c.onErrorContainer
        "warning" -> c.tertiaryContainer to c.onTertiaryContainer
        "success" -> c.primaryContainer to c.onPrimaryContainer
        else -> c.surfaceContainerHighest to c.onSurface
    }
}

@Composable
private fun toneAccent(tone: String): Color {
    val c = MaterialTheme.colorScheme
    return when (tone) {
        "danger" -> c.error
        "warning" -> c.tertiary
        "success" -> c.primary
        else -> c.secondary
    }
}

/**
 * 聊天界面顶部的实时状态区（由故事包的 `hud:` 定义）。
 *
 * 紧凑模式：宽字段（如“主线”）一行 + 其余紧凑字段的信息条；点击展开为完整卡片（全部字段、进度条、事件提示）。
 * 顶栏标题已经显示地点，所以紧凑信息条里不再重复“位置”。
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun HudBar(
    hud: List<HudFieldV1>,
    story: StoryBriefV1?,
    modifier: Modifier = Modifier,
    initiallyExpanded: Boolean = false,
    upcoming: List<WorldEventChipV1> = emptyList(),
    conditions: List<String> = emptyList(),
) {
    if (hud.isEmpty() && story == null && upcoming.isEmpty()) return
    var expanded by rememberSaveable { mutableStateOf(initiallyExpanded) }
    val wide = hud.filter { it.wide && it.compact }
    val chips = hud.filter { !it.wide && it.compact && it.id != "location" }
    val expandLabel = stringResource(R.string.hud_expand)
    val collapseLabel = stringResource(R.string.hud_collapse)
    Surface(color = MaterialTheme.colorScheme.surfaceContainerLow, modifier = modifier.fillMaxWidth()) {
        Column(
            Modifier
                .animateContentSize()
                .clickable(onClickLabel = if (expanded) collapseLabel else expandLabel) { expanded = !expanded }
                .semantics { stateDescription = if (expanded) collapseLabel else expandLabel }
                .padding(top = 6.dp, bottom = 8.dp),
        ) {
            Row(Modifier.padding(start = 16.dp, end = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    if (wide.isEmpty()) {
                        Text(
                            stringResource(R.string.hud_title), style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    wide.forEach { f -> WideLine(f, expanded) }
                }
                Icon(
                    if (expanded) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                    contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 4.dp).size(20.dp),
                )
            }
            if (!expanded && (chips.isNotEmpty() || upcoming.isNotEmpty() || conditions.isNotEmpty())) {
                LazyRow(
                    contentPadding = PaddingValues(horizontal = 16.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    modifier = Modifier.padding(top = 6.dp),
                ) {
                    items(chips, key = { it.id }) { HudChip(it) }
                    items(conditions, key = { "c:$it" }) { ConditionChip(it) }
                    items(upcoming, key = { "e:" + it.id }) { EventChip(it) }
                }
            }
            AnimatedVisibility(expanded, enter = expandVertically(), exit = shrinkVertically()) {
                Column(Modifier.padding(horizontal = 16.dp).padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    FlowRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                        maxItemsInEachRow = 2,
                    ) {
                        hud.filter { !it.wide }.forEach { f -> HudTile(f, Modifier.weight(1f)) }
                    }
                    story?.takeIf { it.hints.isNotEmpty() }?.let { st ->
                        HorizontalDivider()
                        Row(verticalAlignment = Alignment.Top) {
                            Icon(Icons.Outlined.Lightbulb, contentDescription = null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.tertiary)
                            Spacer(Modifier.width(8.dp))
                            Column {
                                Text(stringResource(R.string.game_story, st.title), style = MaterialTheme.typography.labelLarge)
                                Text(stringResource(R.string.game_hints_title), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                st.hints.forEach { Text("· $it", style = MaterialTheme.typography.bodySmall) }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun WideLine(f: HudFieldV1, expanded: Boolean) {
    val accent = toneAccent(f.tone)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.clearAndSetSemantics { contentDescription = "${f.label}：${f.value}" },
    ) {
        Icon(symbolFor(f.icon), contentDescription = null, tint = accent, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(8.dp))
        Text(f.label, style = MaterialTheme.typography.labelLarge, color = accent)
        Spacer(Modifier.width(8.dp))
        Text(
            f.value, style = MaterialTheme.typography.bodyMedium,
            maxLines = if (expanded) 4 else 1, overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun HudChip(f: HudFieldV1) {
    val (bg, fg) = toneColors(f.tone)
    Surface(
        color = bg, contentColor = fg, shape = MaterialTheme.shapes.small,
        modifier = Modifier.heightIn(min = 32.dp).clearAndSetSemantics { contentDescription = "${f.label}：${f.value}" },
    ) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(symbolFor(f.icon), contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(f.label, style = MaterialTheme.typography.labelMedium, color = fg.copy(alpha = 0.78f))
            Spacer(Modifier.width(4.dp))
            Text(f.value, style = MaterialTheme.typography.labelLarge, maxLines = 1)
        }
    }
}

@Composable
private fun HudTile(f: HudFieldV1, modifier: Modifier = Modifier) {
    val (bg, fg) = toneColors(f.tone)
    Surface(
        color = bg, contentColor = fg, shape = MaterialTheme.shapes.medium,
        modifier = modifier.clearAndSetSemantics { contentDescription = "${f.label}：${f.value}" },
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(symbolFor(f.icon), contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text(f.label, style = MaterialTheme.typography.labelMedium, color = fg.copy(alpha = 0.78f), maxLines = 1)
            }
            Text(f.value, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
            if (f.progress >= 0) {
                LinearProgressIndicator(
                    progress = { f.progress / 1000f },
                    color = toneAccent(f.tone),
                    trackColor = fg.copy(alpha = 0.16f),
                    modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
                )
            }
        }
    }
}

/** 状态（擦伤、中毒…）：errorContainer。 */
@Composable
private fun ConditionChip(text: String) {
    val c = MaterialTheme.colorScheme
    Surface(color = c.errorContainer, contentColor = c.onErrorContainer, shape = MaterialTheme.shapes.small, modifier = Modifier.heightIn(min = 32.dp)) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.Healing, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(text, style = MaterialTheme.typography.labelLarge, maxLines = 1)
        }
    }
}

/** 世界事件倒计时：tertiaryContainer；只听过传闻的事件用喇叭图标。 */
@Composable
private fun EventChip(e: WorldEventChipV1) {
    val c = MaterialTheme.colorScheme
    Surface(
        color = c.tertiaryContainer, contentColor = c.onTertiaryContainer, shape = MaterialTheme.shapes.small,
        modifier = Modifier.heightIn(min = 32.dp).clearAndSetSemantics { contentDescription = "世界事件 ${e.title}：${e.countdown ?: e.statusLabel}" },
    ) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(if (e.rumored) Icons.Outlined.Campaign else Icons.Outlined.Event, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(e.title, style = MaterialTheme.typography.labelLarge, maxLines = 1)
            (e.countdown ?: e.statusLabel).takeIf { it.isNotBlank() }?.let {
                Spacer(Modifier.width(4.dp))
                Text("· $it", style = MaterialTheme.typography.labelMedium, color = c.onTertiaryContainer.copy(alpha = 0.8f), maxLines = 1)
            }
        }
    }
}
