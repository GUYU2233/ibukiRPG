package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Bookmark
import androidx.compose.material.icons.outlined.BookmarkAdd
import androidx.compose.material.icons.outlined.CallSplit
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.History
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.guyu2233.ibukirpg.app.data.BranchV1
import com.guyu2233.ibukirpg.app.data.CheckpointV1
import com.guyu2233.ibukirpg.app.data.TimelineTurnV1
import com.guyu2233.ibukirpg.app.data.TimelineV1

/** 时间线与检查点的操作。 */
data class TimelineActions(
    val onClose: () -> Unit = {},
    val onRollback: (Int) -> Unit = {},
    val onCheckpointAt: (Int) -> Unit = {},
    val onRestore: (CheckpointV1) -> Unit = {},
    val onSwitch: (BranchV1) -> Unit = {},
    val onNewCheckpoint: () -> Unit = {},
)

@Composable
fun TimelineDialog(t: TimelineV1, saveName: String, subtitle: String, actions: TimelineActions) {
    Dialog(onDismissRequest = actions.onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
            TimelineContent(t, saveName, subtitle, actions)
        }
    }
}

private sealed interface Row0 {
    val turn: Int
    data class Turn(val t: TimelineTurnV1) : Row0 { override val turn get() = t.turn }
    data class Cp(val c: CheckpointV1) : Row0 { override val turn get() = c.turn }
    data class Fork(val b: BranchV1, val at: Int) : Row0 { override val turn get() = at }
}

/** 时间线的行（回合倒序；检查点排在同号回合之上；分叉的兄弟分支挂在分叉点下面）。纯函数，便于测试。 */
private fun rows(t: TimelineV1): List<Row0> {
    val cur = t.branches.firstOrNull { it.current }
    // 当前分支沿父链可见的检查点：自己的，或父分支上不晚于分叉点的
    val lineage = mutableMapOf<String, Int>()
    var b = cur
    var limit = Int.MAX_VALUE
    while (b != null) {
        lineage[b.id] = limit
        limit = b.forkTurn
        b = t.branches.firstOrNull { it.id == b!!.parent }
    }
    val cps = t.checkpoints.filter { c -> lineage[c.branch]?.let { c.turn <= it } ?: false }
    val forks: List<Pair<Int, BranchV1>> = t.branches.filter { it.id != cur?.id }.mapNotNull { x ->
        when {
            cur == null -> null
            x.parent == cur.id -> x.forkTurn to x
            x.id == cur.parent -> cur.forkTurn to x
            x.parent != null && x.parent == cur.parent -> x.forkTurn to x
            else -> null
        }
    }
    val out = mutableListOf<Row0>()
    t.turns.forEach { turn ->
        cps.filter { it.turn == turn.turn }.forEach { out += Row0.Cp(it) }
        out += Row0.Turn(turn)
        forks.filter { it.first == turn.turn }.forEach { out += Row0.Fork(it.second, it.first) }
    }
    return out
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TimelineContent(t: TimelineV1, saveName: String, subtitle: String, actions: TimelineActions, initialSelected: Int? = null) {
    var selected by rememberSaveable { mutableStateOf(initialSelected) }
    val list = remember(t) { rows(t) }
    val c = MaterialTheme.colorScheme
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("时间线与检查点", style = MaterialTheme.typography.titleLarge)
                        Text("$subtitle · 存档「$saveName」", style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant)
                    }
                },
                navigationIcon = { IconButton(onClick = actions.onClose) { Icon(Icons.Outlined.Close, contentDescription = "关闭") } },
            )
        },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = actions.onNewCheckpoint,
                icon = { Icon(Icons.Outlined.BookmarkAdd, contentDescription = null) },
                text = { Text("新建检查点") },
            )
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            Row(
                Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp, vertical = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                t.branches.sortedByDescending { it.current }.forEach { b ->
                    FilterChip(
                        selected = b.current,
                        onClick = { if (!b.current) actions.onSwitch(b) },
                        leadingIcon = { Icon(Icons.Outlined.CallSplit, contentDescription = null, modifier = Modifier.size(FilterChipDefaults.IconSize)) },
                        label = {
                            Text(
                                buildString {
                                    append(b.name)
                                    if (b.current) append(" · 当前") else append(" · ${b.headTurn} 回合")
                                    if (b.status == "rolled_back" && !b.current) append(" · 已回溯")
                                },
                            )
                        },
                    )
                }
            }
            if (t.pendingTurn != 0) {
                Text(
                    "已选择回到回合 ${maxOf(t.pendingTurn, 0)}：下一次行动会从这里开出新分支（原来的进展会保留）。",
                    style = MaterialTheme.typography.bodySmall, color = c.tertiary,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                )
            }
            LazyColumn(contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 96.dp)) {
                items(list, key = {
                    when (it) {
                        is Row0.Turn -> "t${it.t.turn}"
                        is Row0.Cp -> "c${it.c.id}"
                        is Row0.Fork -> "f${it.b.id}"
                    }
                }) { r ->
                    when (r) {
                        is Row0.Turn -> TurnRow(r.t, selected == r.t.turn, onClick = { selected = if (selected == r.t.turn) null else r.t.turn }, actions)
                        is Row0.Cp -> CheckpointRow(r.c, onClick = { actions.onRestore(r.c) })
                        is Row0.Fork -> ForkRow(r.b, r.at, onSwitch = { actions.onSwitch(r.b) })
                    }
                }
            }
        }
    }
}

/** 左侧的竖线 + 节点。 */
@Composable
private fun Rail(node: @Composable () -> Unit, dashed: Boolean = false) {
    val color = MaterialTheme.colorScheme.primary
    Box(Modifier.width(28.dp).fillMaxHeight(), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            drawLine(color, Offset(size.width / 2, 0f), Offset(size.width / 2, size.height), strokeWidth = 3.dp.toPx())
            if (dashed) {
                drawLine(
                    color.copy(alpha = 0.6f), Offset(size.width / 2 + 10.dp.toPx(), 0f), Offset(size.width / 2 + 10.dp.toPx(), size.height),
                    strokeWidth = 1.5.dp.toPx(), pathEffect = PathEffect.dashPathEffect(floatArrayOf(8f, 8f)),
                )
            }
        }
        node()
    }
}

@Composable
private fun Dot(filled: Boolean, big: Boolean = false, square: Boolean = false, color: Color = MaterialTheme.colorScheme.primary) {
    val bg = MaterialTheme.colorScheme.background
    Canvas(Modifier.size(if (big) 18.dp else 14.dp)) {
        if (square) {
            drawRect(color)
            return@Canvas
        }
        drawCircle(bg)
        if (filled) drawCircle(color) else drawCircle(color, style = Stroke(width = 2.dp.toPx()))
    }
}

@Composable
private fun TurnRow(t: TimelineTurnV1, selected: Boolean, onClick: () -> Unit, actions: TimelineActions) {
    val c = MaterialTheme.colorScheme
    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
        Rail({ Dot(filled = t.current, big = t.current || t.fork) })
        Spacer(Modifier.width(8.dp))
        Surface(
            onClick = onClick,
            shape = RoundedCornerShape(14.dp),
            color = if (selected) c.surfaceContainerLowest else Color.Transparent,
            border = if (selected) BorderStroke(1.dp, c.outlineVariant) else null,
            modifier = Modifier.weight(1f).padding(vertical = 4.dp),
        ) {
            Column(Modifier.padding(horizontal = 10.dp, vertical = 8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(if (t.turn == 0) "开场" else "回合 ${t.turn}", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                    if (t.current) { Spacer(Modifier.width(8.dp)); Badge("当前", c.primary, c.onPrimary) }
                    if (t.fork) { Spacer(Modifier.width(8.dp)); Badge("分叉点", c.tertiaryContainer, c.onTertiaryContainer) }
                    Spacer(Modifier.weight(1f))
                    t.time?.let { Text(it, style = MaterialTheme.typography.labelSmall, color = c.onSurfaceVariant) }
                }
                Text(t.summary, style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant, maxLines = 2)
                if (selected && !t.current) {
                    Row(Modifier.padding(top = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilledTonalButton(onClick = { actions.onRollback(t.turn) }) {
                            Icon(Icons.Outlined.History, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(6.dp))
                            Text("回到这里")
                        }
                        TextButton(onClick = { actions.onCheckpointAt(t.turn) }) {
                            Icon(Icons.Outlined.BookmarkAdd, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(6.dp))
                            Text("设为检查点")
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun CheckpointRow(cp: CheckpointV1, onClick: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min).clickable(onClickLabel = "恢复到这个检查点", onClick = onClick)) {
        Rail({ Dot(filled = true, square = true, color = c.tertiary) })
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f).padding(horizontal = 10.dp, vertical = 10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.Bookmark, contentDescription = null, tint = c.tertiary, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("检查点「${cp.name}」", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                Text(
                    when (cp.kind) { "auto" -> "自动"; "daily" -> "每日"; else -> "手动" },
                    style = MaterialTheme.typography.labelSmall, color = c.onSurfaceVariant,
                )
            }
            Text(
                "回合 ${cp.turn}" + (cp.reason?.let { " · $it" } ?: ""),
                style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun ForkRow(b: BranchV1, at: Int, onSwitch: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
        Rail({}, dashed = true)
        Spacer(Modifier.width(18.dp))
        Column(Modifier.weight(1f).padding(vertical = 8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.CallSplit, contentDescription = null, tint = c.onSurfaceVariant, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("${b.name} · 回合 ${at + 1}–${b.headTurn}", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                TextButton(onClick = onSwitch) { Text("切换") }
            }
            Text(
                if (b.status == "rolled_back") "这条线已被回溯，但完整保留，随时可以切回去。" else "另一条进行中的分支。",
                style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant,
            )
        }
    }
}
