# Run after assemble-index: footage must sit BELOW the frame hosts (text/plates are inside the hosts),
# ids must be unique, and the music fades out over the last 2.2s.
import re
p='index.html'; s=open(p).read()
vids=re.findall(r'[ \t]*<video id="el-[^"]*-video-\d+".*?</video>\n?',s,flags=re.S)
for v in vids: s=s.replace(v,'',1)
out=[]
for i,v in enumerate(vids,1):
    out.append(re.sub(r'id="el-[^"]*-video-\d+"',f'id="el-footage-{i}"',v,count=1))
marker=s.index('<div\n        id="el-01-hook"') if '<div\n        id="el-01-hook"' in s else None
if marker is None:
    m=re.search(r'<div[^>]*\n?\s*id="el-01-hook"',s); marker=m.start()
s=s[:marker]+''.join(out)+'\n      '+s[marker:]
auto="""data-automation='{"version":1,"lanes":[{"target":"volume","points":[{"t":0,"v":1},{"t":34.8,"v":1},{"t":37,"v":0}]}]}'"""
if 'data-automation' not in s:
    s=s.replace('        data-volume="0.9"\n      ></audio>','        data-volume="0.9"\n        '+auto+'\n      ></audio>',1)
open(p,'w').write(s)
print(len(vids),'footage clips moved')
