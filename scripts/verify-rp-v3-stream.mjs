import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { execFileSync } from 'node:child_process'
import ts from 'typescript'
const source = await readFile(new URL('../src/lib/narrativeStream.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText
const { validateNarrativeComposition, readNarrativeStream } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'))
const db = process.argv[2]
assert.ok(db, 'pass disposable database path')
const rows = JSON.parse(execFileSync('sqlite3', ['-readonly', '-json', db, "SELECT narrative_json,narrative_composition_version,narrative_fact_groups_json,narrative_fact_event_ids_json FROM rp_turn_runs ORDER BY rowid LIMIT 1"], { encoding: 'utf8' }))
const saved = rows[0]
const view = { lines: JSON.parse(saved.narrative_json), composition_version: saved.narrative_composition_version, fact_groups: JSON.parse(saved.narrative_fact_groups_json), event_ids: JSON.parse(saved.narrative_fact_event_ids_json) }
assert.equal(view.composition_version, 'corerp.fact-composition.v3')
validateNarrativeComposition(view)
const frames = view.lines.map((line,index)=>({type:'line',chunk:{index,line,event_ids:view.fact_groups[index]}}))
const done = {type:'done',count:view.lines.length,event_ids:view.event_ids,warnings:[],composition_version:view.composition_version,fact_groups:view.fact_groups}
const stream = values => new Response(values.map(x=>JSON.stringify(x)+'\n').join(''), {headers:{'Content-Type':'application/x-ndjson'}})
assert.deepEqual((await readNarrativeStream(stream([...frames,done]),()=>{})).view,{...view,warnings:[]})
for (const invalid of [ {...view,composition_version:'corerp.fact-composition.v4'}, {...view,event_ids:undefined}, {...view,event_ids:[...view.event_ids].reverse()}, {...view,fact_groups:view.fact_groups.map(()=>[view.event_ids[0]])} ]) assert.throws(()=>validateNarrativeComposition(invalid))
for (const invalid of [ [...frames, {...done,count:done.count+1}], [...frames,done,done], frames, [...frames,{...done,event_ids:[...done.event_ids,'invented']}], [{...frames[0],chunk:{...frames[0].chunk,index:1}},...frames.slice(1),done] ]) await assert.rejects(()=>readNarrativeStream(stream(invalid),()=>{}))
validateNarrativeComposition({lines:['旧叙述。'],composition_version:'corerp.fact-composition.v1',fact_groups:[['old']]})
validateNarrativeComposition({lines:['旧正文。']})
const groupFrames = [{type:'line',chunk:{index:0,line:'甲。',event_ids:['a','b']}},{type:'line',chunk:{index:1,line:'乙。',event_ids:['c']}}]
await assert.rejects(()=>readNarrativeStream(stream([...groupFrames,{type:'done',count:2,event_ids:['a','b','c'],warnings:[],composition_version:'corerp.fact-composition.v3',fact_groups:[['a'],['b','c']]}]),()=>{}))
const empty = {lines:[],event_ids:[],fact_groups:[],composition_version:'corerp.fact-composition.v2'}
validateNarrativeComposition(empty)
assert.throws(()=>validateNarrativeComposition({...empty,composition_version:'corerp.fact-composition.v3'}))
console.log(JSON.stringify({status:'PASS',source:'actual first accepted v3 turn; no model calls',savedAndStream:true,negativeChecks:11}))
