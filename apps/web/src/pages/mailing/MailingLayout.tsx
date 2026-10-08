import { NavLink, Outlet } from 'react-router-dom'
import { useI18n } from '../../i18n'

export default function MailingLayout() {
  const { t } = useI18n()
  const tab = ({ isActive }: { isActive: boolean }) =>
    `rounded-lg px-3 py-1.5 border text-sm ${isActive ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-[var(--line)] text-[var(--muted)]'}`
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">{t('mailing.title')}</h2>
        <p className="text-sm text-[var(--muted)]">{t('mailing.subtitle')}</p>
      </div>
      <nav className="flex flex-wrap gap-2">
        <NavLink to="/mailing/contacts" className={tab}>{t('mailing.contacts')}</NavLink>
      </nav>
      <Outlet />
    </div>
  )
}
