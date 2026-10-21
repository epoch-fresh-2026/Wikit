import { useState } from 'react'
import { useAction } from '@/lib/hooks'
import { ActionFeedback, Field } from '@/components/shared'
import { Button } from '@/components/ui/button'

export function Moderation({
  path,
  hidden,
  version,
  done,
}: {
  path: string
  hidden: boolean
  version: number
  done: () => void
}) {
  const [reason, setReason] = useState(''),
    action = useAction()
  return (
    <details>
      <summary>内容治理：{hidden ? '恢复' : '隐藏'}</summary>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void action.run(async () => {
            await action.write(path, { hidden: !hidden, reason, expected_version: version })
            done()
          })
        }}
      >
        <Field
          label="治理理由"
          value={reason}
          onChange={setReason}
          required
          hint="需要当前 manage 权限；操作及理由会写入审计。"
        />
        <Button variant="outline" disabled={action.busy}>
          {hidden ? '恢复内容' : '隐藏内容'}
        </Button>
        <ActionFeedback action={action} />
      </form>
    </details>
  )
}
