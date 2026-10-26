import { useTranslation } from 'react-i18next';
import { Card, Empty, Tag, Typography } from 'antd';

import type { ShopPlan } from '@/generated/zod';
import { formatMoney } from '@/lib/money';

const BYTES_PER_GB = 1024 ** 3;

interface PlanListProps {
  plans: ShopPlan[];
  selected: number | null;
  onSelect: (id: number) => void;
}

// Plans keep the catalog order; a category appears where its first plan does.
export function groupByCategory(plans: ShopPlan[]): { name: string; plans: ShopPlan[] }[] {
  const groups: { name: string; plans: ShopPlan[] }[] = [];
  for (const p of plans) {
    const group = groups.find((g) => g.name === p.category);
    if (group) group.plans.push(p);
    else groups.push({ name: p.category, plans: [p] });
  }
  return groups;
}

export default function PlanList({ plans, selected, onSelect }: PlanListProps) {
  const { t } = useTranslation();
  if (plans.length === 0) return <Empty description={t('shop.noPlans')} />;
  const groups = groupByCategory(plans);
  return (
    <>
      {groups.map((group) => (
        <section key={group.name}>
          {groups.length > 1 && (
            <Typography.Title level={5} dir="auto">
              {group.name || t('shopbot.app.categoryOther')}
            </Typography.Title>
          )}
          <div className="shop-plans">
            {group.plans.map((p) => (
              <PlanCard key={p.id} plan={p} selected={p.id === selected} onSelect={onSelect} />
            ))}
          </div>
        </section>
      ))}
    </>
  );
}

function PlanCard({
  plan: p,
  selected,
  onSelect,
}: {
  plan: ShopPlan;
  selected: boolean;
  onSelect: (id: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card
      size="small"
      hoverable
      className={selected ? 'shop-plan is-selected' : 'shop-plan'}
      onClick={() => onSelect(p.id)}
    >
      <div className="shop-plan-name" dir="auto">
        {p.name}
      </div>
      <div className="shop-plan-price">{formatMoney(p.price, p.currency)}</div>
      {p.description && (
        <div className="shop-plan-desc" dir="auto">
          {p.description}
        </div>
      )}
      <div className="shop-plan-facts">
        <Tag>
          {p.totalGB === 0
            ? t('shop.unlimitedData')
            : t('shop.dataGb', {
                gb: Math.round((p.totalGB / BYTES_PER_GB) * 100) / 100,
              })}
        </Tag>
        <Tag>
          {p.durationDays === 0 ? t('shop.noExpiry') : t('shop.days', { days: p.durationDays })}
        </Tag>
        {p.limitIp > 0 && <Tag>{t('shop.devices', { count: p.limitIp })}</Tag>}
      </div>
    </Card>
  );
}
