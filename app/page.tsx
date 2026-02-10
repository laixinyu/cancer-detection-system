'use client'

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/components/i18n-provider";
import LanguageSwitcher from "@/components/language-switcher";

export default function Home() {
  const { t } = useI18n()

  return (
    <div className="min-h-screen bg-gradient-to-b from-blue-50 to-white">
      <div className="container mx-auto px-4 py-16">
        <div className="flex justify-end mb-6">
          <LanguageSwitcher />
        </div>
        <div className="text-center max-w-4xl mx-auto">
          <h1 className="text-5xl font-bold text-gray-900 mb-6">
            {t('home.title')}
          </h1>
          <p className="text-xl text-gray-600 mb-8">
            {t('home.subtitle')}
          </p>
          
          <div className="flex gap-4 justify-center mb-16">
            <Link href="/login">
              <Button size="lg">
                {t('home.getStarted')}
              </Button>
            </Link>
            <Link href="/register">
              <Button size="lg" variant="outline">
                {t('home.createAccount')}
              </Button>
            </Link>
          </div>

          <div className="grid md:grid-cols-3 gap-8 mt-16">
            <div className="p-6 bg-white rounded-lg shadow-md">
              <div className="text-4xl mb-4">🔬</div>
              <h3 className="text-xl font-semibold mb-2">{t('home.aiDetection')}</h3>
              <p className="text-gray-600">
                {t('home.aiDetectionDesc')}
              </p>
            </div>

            <div className="p-6 bg-white rounded-lg shadow-md">
              <div className="text-4xl mb-4">👨‍⚕️</div>
              <h3 className="text-xl font-semibold mb-2">{t('home.doctorReview')}</h3>
              <p className="text-gray-600">
                {t('home.doctorReviewDesc')}
              </p>
            </div>

            <div className="p-6 bg-white rounded-lg shadow-md">
              <div className="text-4xl mb-4">📊</div>
              <h3 className="text-xl font-semibold mb-2">{t('home.reports')}</h3>
              <p className="text-gray-600">
                {t('home.reportsDesc')}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
